"""CPU / RPS 真实伸缩验收：负载上升、回落与操作快照。"""

import time

from common import Load, wait_for


def run(env, target):
    results = {}
    for metric, work in [("rps", 0), ("cpu_utilization", 20000)]:
        wait_for(
            metric + " valid",
            lambda: env.workload_state()["rps" if metric == "rps" else "cpuUtilization"]["quality"]
            == "ok",
        )
        if env.policy is None:
            env.create_policy(metric)
        else:
            current = env.api("policies/" + env.policy["id"])
            current.update(metric=metric, targetValue=0.2)
            env.policy = env.api("policies/" + current["id"], current, "PUT")
        started = time.monotonic()
        with Load(target, work) as load:
            wait_for(metric + " scale up", lambda: env.workload_state()["desiredReplicas"] == 3)
            write_seconds = round(time.monotonic() - started, 1)
            wait_for(
                metric + " ready after scale up", lambda: env.workload_state()["readyReplicas"] == 3
            )
        ready_seconds = round(time.monotonic() - started, 1)
        started = time.monotonic()
        wait_for(metric + " scale down", lambda: env.workload_state()["desiredReplicas"] == 1)
        down_seconds = round(time.monotonic() - started, 1)
        wait_for(
            metric + " ready after scale down", lambda: env.workload_state()["readyReplicas"] == 1
        )
        records = [
            op
            for op in env.operations()
            if (op.get("policy") or {}).get("id") == env.policy["id"]
            and op.get("metricId") == metric
            and op["status"] == "succeeded"
        ]
        assert any(op["type"] == "自动扩容" for op in records), records
        assert any(op["type"] == "自动缩容" for op in records), records
        assert all(
            op.get("metricSampledAt")
            and op["actualReplicas"] == op["desiredReplicas"]
            and 1 <= op["desiredReplicas"] <= 3
            for op in records
        ), records
        results[metric] = dict(
            scaleUpWriteSeconds=write_seconds,
            scaleUpReadySeconds=ready_seconds,
            scaleDownWriteSeconds=down_seconds,
            **load.counts,
        )
    return results
