#!/usr/bin/env python3
"""阶段二真实集群验收，仅对独立临时命名空间执行伸缩与 HPA 冲突测试。

要求：独立后端、已采集指标的 demo-custom、可跨 Pod 替换访问的 TARGET。
运行前确认目标没有其他策略；结束时暂停并删除本脚本的策略和 HPA。
"""
import json
import os
from pathlib import Path
import subprocess
import threading
import time
import urllib.error
import urllib.request

API = os.environ.get('API_URL', 'http://127.0.0.1:18080').rstrip('/')
NAMESPACE = os.environ.get('TEST_NAMESPACE', 'kubescale-phase2-test')
WORKLOAD = os.environ.get('TEST_WORKLOAD', 'demo-custom')
TARGET = os.environ['TARGET'].rstrip('/')
OUT = Path(os.environ.get('RESULT_DIR', 'tests/results/phase2'))
if not NAMESPACE.startswith('kubescale-phase2-'):
    raise RuntimeError('use a disposable kubescale-phase2-* namespace')
OUT.mkdir(parents=True, exist_ok=True)


def api(path, data=None, method=None, expected=200):
    request = urllib.request.Request(
        API + '/api/v1/' + path,
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


def kubectl(*args, data=None):
    return subprocess.check_output(['kubectl', '-n', NAMESPACE, *args], input=data)


def wait_for(label, predicate, timeout=240):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        value = predicate()
        if value:
            print(label, value, flush=True)
            return value
        time.sleep(3)
    raise RuntimeError(label + ' timed out')


def workload():
    return api(f'workloads/{NAMESPACE}/{WORKLOAD}')


def operations():
    return api(f'operations?namespace={NAMESPACE}&workload={WORKLOAD}&pageSize=100')['items']


stop = threading.Event()
threads = []
counts = {'requests': 0, 'errors': 0}
lock = threading.Lock()


def worker():
    while not stop.is_set():
        key = 'requests'
        try:
            with urllib.request.urlopen(TARGET, timeout=5) as response:
                response.read()
        except Exception:
            key = 'errors'
        with lock:
            counts[key] += 1
        stop.wait(.03)


policy = None
hpa_created = False
results = {}
try:
    if api('policies')['total']:
        raise RuntimeError('test backend must have no existing policies')
    api(f'workloads/{NAMESPACE}/{WORKLOAD}/scale', {'replicas': 1, 'reason': 'phase2 baseline'}, 'POST', 202)
    wait_for('ready baseline', lambda: workload()['readyReplicas'] == 1)
    wait_for('valid metrics', lambda: workload()['rps']['quality'] == 'ok')
    draft = dict(name='phase2-acceptance', namespace=NAMESPACE, workloadName=WORKLOAD,
                 metric='rps', targetValue=10, minReplicas=1, maxReplicas=3,
                 scaleUpConsecutivePeriods=2, scaleDownConsecutivePeriods=2,
                 cooldownSeconds=10, stabilizationSeconds=20,
                 samplingIntervalSeconds=5, status='enabled')
    policy = api('policies', draft, 'POST', 201)
    start = time.time()
    threads = [threading.Thread(target=worker) for _ in range(4)]
    for thread in threads:
        thread.start()
    wait_for('RPS scale up', lambda: workload()['desiredReplicas'] == 3)
    results['scaleUpSeconds'] = round(time.time() - start, 1)
    stop.set()
    for thread in threads:
        thread.join()
    start = time.time()
    wait_for('RPS scale down', lambda: workload()['desiredReplicas'] == 1)
    results['scaleDownSeconds'] = round(time.time() - start, 1)
    wait_for('ready after scale down', lambda: workload()['readyReplicas'] == 1)
    hpa = {'apiVersion': 'autoscaling/v2', 'kind': 'HorizontalPodAutoscaler',
           'metadata': {'name': 'phase2-conflict', 'namespace': NAMESPACE},
           'spec': {'scaleTargetRef': {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'name': WORKLOAD},
                    'minReplicas': 1, 'maxReplicas': 1,
                    'metrics': [{'type': 'Resource', 'resource': {'name': 'cpu', 'target': {'type': 'Utilization', 'averageUtilization': 50}}}]}}
    kubectl('create', '-f', '-', data=json.dumps(hpa).encode())
    hpa_created = True
    wait_for('running policy rejects HPA takeover', lambda: any(
        op.get('errorCode') == 'CONTROL_CONFLICT' for op in operations()))
    current = api('policies/' + policy['id'])
    paused = api('policies/' + policy['id'] + '/status', {'status': 'paused', 'version': current['version']}, 'PATCH')
    conflict = api('policies/' + policy['id'] + '/status', {'status': 'enabled', 'version': paused['version']}, 'PATCH', 409)
    assert conflict['code'] == 'CONTROL_CONFLICT', conflict
    results['hpaConflict'] = True
    records = operations()
    scaled = [op for op in records if op['status'] == 'succeeded' and op['source'] == 'KubeScale']
    assert any(op['type'] == '自动扩容' for op in scaled)
    assert any(op['type'] == '自动缩容' for op in scaled)
    assert all(op['policy']['id'] == policy['id'] and op.get('metricSampledAt') and op['actualReplicas'] == op['desiredReplicas'] for op in scaled)
    assert all(1 <= op['desiredReplicas'] <= 3 for op in scaled)
    OUT.joinpath('operations.json').write_text(json.dumps(records, indent=2, ensure_ascii=False))
    results['passed'] = True
except Exception as error:
    results.update(passed=False, error=str(error))
    raise
finally:
    stop.set()
    for thread in threads:
        thread.join()
    try:
        if policy:
            current = api('policies/' + policy['id'])
            api('policies/' + policy['id'] + '/status', {'status': 'paused', 'version': current['version']}, 'PATCH')
            api('policies/' + policy['id'], method='DELETE', expected=204)
        if hpa_created:
            kubectl('delete', 'hpa', 'phase2-conflict')
    finally:
        results.update(counts)
        OUT.joinpath('acceptance.json').write_text(json.dumps(results, indent=2))
        print(json.dumps(results), flush=True)
