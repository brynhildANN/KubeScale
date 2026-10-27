"""HPA 控制权验收：运行中拒绝接管，存在 HPA 时拒绝启用策略。"""
import json
from common import wait_for


def run(env):
    env.create_policy()
    hpa = {'apiVersion': 'autoscaling/v2', 'kind': 'HorizontalPodAutoscaler',
           'metadata': {'name': env.hpa_name, 'namespace': env.namespace},
           'spec': {'scaleTargetRef': {'apiVersion': 'apps/v1', 'kind': 'Deployment', 'name': env.workload},
                    'minReplicas': 1, 'maxReplicas': 1,
                    'metrics': [{'type': 'Resource', 'resource': {'name': 'cpu', 'target': {
                        'type': 'Utilization', 'averageUtilization': 50}}}]}}
    env.hpa_created = True
    env.kubectl('create', '-f', '-', data=json.dumps(hpa).encode())
    wait_for('running policy rejects HPA takeover', lambda: any(
        (op.get('policy') or {}).get('id') == env.policy['id'] and op.get('errorCode') == 'CONTROL_CONFLICT'
        for op in env.operations()))
    current = env.api('policies/' + env.policy['id'])
    paused = env.api('policies/' + current['id'] + '/status',
                     {'status': 'paused', 'version': current['version']}, 'PATCH')
    conflict = env.api('policies/' + current['id'] + '/status',
                       {'status': 'enabled', 'version': paused['version']}, 'PATCH', 409)
    assert conflict['code'] == 'CONTROL_CONFLICT', conflict
    return {'runningPolicyConflict': True, 'enableConflict': True}
