"""真实集群验收共用工具；仅允许独立的 kubescale-test-* 命名空间。"""
import json
import os
from pathlib import Path
import signal
import subprocess
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

ROOT = Path(__file__).resolve().parents[2]


def wait_for(label, predicate, timeout=240):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            print(label, value, flush=True)
            return value
        time.sleep(3)
    raise RuntimeError(label + ' timed out')


class Environment:
    def __init__(self):
        self.api_url = os.environ.get('API_URL', 'http://127.0.0.1:8080').rstrip('/')
        self.namespace = os.environ.get('TEST_NAMESPACE', 'kubescale-test-e2e')
        self.workload = os.environ.get('TEST_WORKLOAD', 'demo-custom')
        if not self.namespace.startswith('kubescale-test-'):
            raise ValueError('TEST_NAMESPACE must be a disposable kubescale-test-* namespace')
        self.output = Path(os.environ.get('RESULT_DIR', str(ROOT / 'tests/results/e2e')))
        self.output.mkdir(parents=True, exist_ok=True)
        self.policy = None
        self.original_replicas = None
        self.hpa_name = 'acceptance-conflict-' + uuid.uuid4().hex[:10]
        self.hpa_created = False

    @property
    def workload_path(self):
        return 'workloads/' + '/'.join(urllib.parse.quote(v, safe='') for v in (self.namespace, self.workload))

    def api(self, path, data=None, method=None, expected=200):
        request = urllib.request.Request(
            self.api_url + '/api/v1/' + path,
            data=None if data is None else json.dumps(data).encode(),
            headers={'Content-Type': 'application/json'}, method=method)
        try:
            response = urllib.request.urlopen(request, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            if response.status != expected:
                raise RuntimeError(f'{path}: expected {expected}, got {response.status}: {raw!r}')
            return json.loads(raw) if raw else None

    def kubectl(self, *args, data=None):
        return subprocess.check_output(
            ['kubectl', '--request-timeout=15s', '-n', self.namespace, *args], input=data, timeout=25)

    def workload_state(self):
        return self.api(self.workload_path)

    def operations(self):
        query = urllib.parse.urlencode(dict(namespace=self.namespace, workload=self.workload, pageSize=100))
        items, page = [], 1
        while True:
            result = self.api(f'operations?{query}&page={page}')
            items.extend(result['items'])
            if len(items) >= result['total'] or not result['items']:
                return items
            page += 1

    def prepare(self):
        # 单独后端/数据库避免已有策略与测试互相干扰。
        if self.api('policies')['total']:
            raise RuntimeError('test backend must have no existing policies')
        hpas = json.loads(self.kubectl('get', 'hpa', '-o', 'json'))['items']
        if any(h['spec']['scaleTargetRef']['name'] == self.workload for h in hpas):
            raise RuntimeError('target already has an HPA')
        self.original_replicas = self.workload_state()['desiredReplicas']
        self.api(self.workload_path + '/scale', {'replicas': 1, 'reason': 'acceptance baseline'}, 'POST', 202)
        wait_for('ready baseline', lambda: self.workload_state()['readyReplicas'] == 1)

    def create_policy(self, metric='rps'):
        draft = dict(name='acceptance-' + uuid.uuid4().hex[:10], namespace=self.namespace,
                     workloadName=self.workload, metric=metric, targetValue=10 if metric == 'rps' else .2,
                     minReplicas=1, maxReplicas=3, scaleUpConsecutivePeriods=2,
                     scaleDownConsecutivePeriods=2, cooldownSeconds=10, stabilizationSeconds=20,
                     samplingIntervalSeconds=5, status='enabled')
        self.policy = self.api('policies', draft, 'POST', 201)
        return self.policy

    def cleanup(self):
        errors = []
        # 独立执行各项清理；一个失败不阻止其他资源的恢复。
        if self.policy:
            try:
                current = self.api('policies/' + self.policy['id'])
                self.api('policies/' + current['id'] + '/status',
                         {'status': 'paused', 'version': current['version']}, 'PATCH')
                self.api('policies/' + current['id'], method='DELETE', expected=204)
            except Exception as error:
                errors.append('policy cleanup: ' + str(error))
        if self.hpa_created:
            try:
                self.kubectl('delete', 'hpa', self.hpa_name, '--ignore-not-found')
            except Exception as error:
                errors.append('HPA cleanup: ' + str(error))
        if self.original_replicas is not None:
            try:
                self.api(self.workload_path + '/scale',
                         {'replicas': self.original_replicas, 'reason': 'acceptance restore'}, 'POST', 202)
            except Exception as error:
                errors.append('replica restore: ' + str(error))
        return errors

    def write(self, name, value):
        (self.output / name).write_text(json.dumps(value, indent=2, ensure_ascii=False) + '\n')


class Load:
    """固定并发请求，不用于声称固定 RPS 或测量系统吞吐上限。"""
    def __init__(self, target, work=0):
        parts = urllib.parse.urlsplit(target)
        query = urllib.parse.parse_qsl(parts.query, keep_blank_values=True)
        query = [(k, v) for k, v in query if k != 'work'] + [('work', str(work))]
        self.target = urllib.parse.urlunsplit(parts._replace(query=urllib.parse.urlencode(query)))
        self.stop = threading.Event()
        self.counts = {'requests': 0, 'errors': 0}
        self.lock = threading.Lock()
        self.threads = []

    def worker(self):
        while not self.stop.is_set():
            key = 'requests'
            try:
                with urllib.request.urlopen(self.target, timeout=5) as response:
                    response.read()
            except Exception:
                key = 'errors'
            with self.lock:
                self.counts[key] += 1
            self.stop.wait(.03)

    def __enter__(self):
        self.threads = [threading.Thread(target=self.worker) for _ in range(4)]
        for thread in self.threads:
            thread.start()
        return self

    def __exit__(self, *_):
        self.stop.set()
        for thread in self.threads:
            thread.join()


def install_signal_handlers():
    def interrupt(*_):
        raise KeyboardInterrupt('acceptance interrupted')
    signal.signal(signal.SIGTERM, interrupt)
