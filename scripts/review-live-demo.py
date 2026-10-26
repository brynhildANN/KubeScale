#!/usr/bin/env python3
"""Replay RPS scaling on the disposable demo workload for visual review.

Uses the existing policy, runs two load/idle cycles, and pauses the policy on
exit. Temporarily exposes the demo Service as NodePort and restores its type.
Run with the backend and Prometheus already available.
"""
import concurrent.futures
import datetime
import json
import os
import signal
import subprocess
import threading
import time
import urllib.request

BASE = os.environ.get('API_URL', 'http://127.0.0.1:8080').rstrip('/') + '/api/v1/'
STOP = threading.Event()

def log(message):
    print(datetime.datetime.now().astimezone().isoformat(timespec='seconds'), message, flush=True)

def api(path, body=None, method=None):
    request = urllib.request.Request(BASE + path,
        data=None if body is None else json.dumps(body).encode(),
        headers={'Content-Type': 'application/json'}, method=method)
    with urllib.request.urlopen(request, timeout=15) as response:
        return json.load(response)

def kubectl(*args):
    return subprocess.check_output(['kubectl', '-n', 'autoscale-demo', *args], text=True)

def interrupt(*_):
    STOP.set()

signal.signal(signal.SIGTERM, interrupt)
signal.signal(signal.SIGINT, interrupt)
policies = api('policies')['items']
policy = next(p for p in policies if p['namespace'] == 'autoscale-demo' and p['workloadName'] == 'demo-custom')
service = json.loads(kubectl('get', 'svc', 'demo-custom', '-o', 'json'))
original_type = service['spec']['type']
policy_id = policy['id']
counts = {'requests': 0, 'errors': 0}

def worker(target, deadline):
    while not STOP.is_set() and time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(target, timeout=5) as response:
                response.read()
            counts['requests'] += 1
        except Exception:
            counts['errors'] += 1
        STOP.wait(.03)

def phase(label, seconds, target=None):
    log(f'{label}; duration={seconds}s')
    deadline = time.monotonic() + seconds
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        futures = [pool.submit(worker, target, deadline) for _ in range(4)] if target else []
        while not STOP.is_set() and time.monotonic() < deadline:
            current = api('policies/' + policy_id)
            if current['status'] != 'enabled':
                log('Policy paused from console; stopping replay')
                STOP.set()
                break
            log('replicas=' + kubectl('get', 'deployment', 'demo-custom', '-o', 'jsonpath={.spec.replicas}'))
            STOP.wait(min(5, max(0, deadline - time.monotonic())))
        for future in futures:
            future.result()

try:
    kubectl('patch', 'svc', 'demo-custom', '-p', '{"spec":{"type":"NodePort"}}')
    service = json.loads(kubectl('get', 'svc', 'demo-custom', '-o', 'json'))
    port = service['spec']['ports'][0]['nodePort']
    node_ip = os.environ.get('NODE_IP') or subprocess.check_output(['minikube', '-p', 'kubescale', 'ip'], text=True).strip()
    target = f'http://{node_ip}:{port}/'
    policy.update(metric='rps', targetValue=10, minReplicas=1, maxReplicas=3,
        scaleUpConsecutivePeriods=2, scaleDownConsecutivePeriods=2,
        samplingIntervalSeconds=5, cooldownSeconds=10, stabilizationSeconds=15,
        status='enabled')
    api('policies/' + policy_id, policy, 'PUT')
    phase('Baseline, no load', 20)
    for index in range(2):
        if STOP.is_set(): break
        phase(f'Round {index + 1}: high RPS, expect scale up', 90, target)
        if STOP.is_set(): break
        phase(f'Round {index + 1}: no load, expect scale down', 150)
finally:
    try:
        current = api('policies/' + policy_id)
        if current['status'] == 'enabled':
            api('policies/' + policy_id + '/status', {'status': 'paused', 'version': current['version']}, 'PATCH')
    finally:
        if original_type == 'ClusterIP':
            kubectl('patch', 'svc', 'demo-custom', '-p', '{"spec":{"type":"ClusterIP","ports":[{"port":80,"nodePort":null}]}}')
    log('Replay finished; policy paused. ' + json.dumps(counts))
