#!/usr/bin/env python3
"""真实集群验收入口；使用 --help 查看配置。"""
import argparse
import json
import os
import signal
import sys
from urllib.parse import urlsplit
from common import Environment, install_signal_handlers
import control_conflict
import scaling


def main():
    parser = argparse.ArgumentParser(description='在独立集群测试环境验证伸缩与控制权；会修改策略、副本和 HPA。',
                                     epilog='配置：API_URL、TEST_NAMESPACE（kubescale-test-*）、TEST_WORKLOAD、TARGET、RESULT_DIR。')
    parser.add_argument('suite', choices=['scaling', 'control-conflict', 'all'])
    args = parser.parse_args()
    target = os.environ.get('TARGET', '')
    if args.suite in ('scaling', 'all') and (urlsplit(target).scheme not in ('http', 'https') or not urlsplit(target).netloc):
        parser.error('伸缩验收需要 TARGET=http(s)://<可跨 Pod 替换访问的 Service 地址>')
    install_signal_handlers()
    suites = ['scaling', 'control-conflict'] if args.suite == 'all' else [args.suite]
    passed = True
    for suite in suites:
        env = Environment()
        result = {'passed': False}
        try:
            env.prepare()
            result['checks'] = scaling.run(env, target) if suite == 'scaling' else control_conflict.run(env)
            result['passed'] = True
        except (Exception, KeyboardInterrupt) as error:
            result['error'] = str(error)
        finally:
            signal.signal(signal.SIGTERM, signal.SIG_IGN)
            signal.signal(signal.SIGINT, signal.SIG_IGN)
            try:
                env.write(suite + '-operations.json', env.operations())
            except Exception as error:
                result['evidenceError'] = str(error)
                result['passed'] = False
            errors = env.cleanup()
            if errors:
                result.update(passed=False, cleanupErrors=errors)
            env.write(suite + '.json', result)
            print(json.dumps(result, ensure_ascii=False), flush=True)
        signal.signal(signal.SIGINT, signal.default_int_handler)
        install_signal_handlers()
        if not result['passed']:
            passed = False
            break
    return 0 if passed else 1


if __name__ == '__main__':
    sys.exit(main())
