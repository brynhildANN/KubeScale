#!/usr/bin/env python3
"""Run against a disposable demo-custom workload with no existing policy.

Requires a running backend, Prometheus scraping every Pod, kubectl context,
and TARGET pointing at a Service endpoint that survives Pod replacement.
Creates an enabled policy and changes replicas; inspect results and pause it
with the API after testing. No third-party Python dependencies required.
"""
import json, os, sys, threading, time, urllib.request, subprocess
from pathlib import Path
OUT=Path(os.environ.get('RESULT_DIR', str(Path(__file__).resolve().parents[1] / 'tests/results')))
OUT.mkdir(parents=True, exist_ok=True)
API=os.environ.get('API_URL','http://127.0.0.1:8080').rstrip('/')
TARGET=os.environ['TARGET'].rstrip('/')
def api(path, data=None, method=None):
 r=urllib.request.Request(API+'/api/v1/'+path, data=None if data is None else json.dumps(data).encode(), headers={'Content-Type':'application/json'},method=method)
 with urllib.request.urlopen(r,timeout=15) as s: return json.load(s)
def replicas():
 return int(subprocess.check_output(['kubectl','-n','autoscale-demo','get','deployment','demo-custom','-o','jsonpath={.spec.replicas}']))
def wait_for(label,predicate,timeout=180):
 end=time.time()+timeout
 while time.time()<end:
  n=replicas(); print(label, n, flush=True)
  if predicate(n): return n
  time.sleep(5)
 raise RuntimeError(label+' timed out')
policy=dict(name='phase1-acceptance',namespace='autoscale-demo',workloadName='demo-custom',metric='rps',targetValue=10,minReplicas=1,maxReplicas=3,scaleUpConsecutivePeriods=2,scaleDownConsecutivePeriods=2,cooldownSeconds=10,stabilizationSeconds=15,samplingIntervalSeconds=5,status='enabled')
p=api('policies',policy,'POST'); OUT.joinpath('policy.json').write_text(json.dumps(p,indent=2))
stop=threading.Event(); count=0; errors=0
work=0
def worker():
 global count,errors
 while not stop.is_set():
  try:
   with urllib.request.urlopen(TARGET+'/?work='+str(work),timeout=5) as r: r.read()
   count+=1
  except Exception: errors+=1
  stop.wait(.03)
def load_on(w=0):
 global work, threads
 work=w; stop.clear(); threads=[threading.Thread(target=worker) for _ in range(4)]
 for t in threads:t.start()
def load_off():
 stop.set()
 for t in threads:t.join()
results={}
try:
 load_on(); results['rps_up']=wait_for('RPS rise',lambda n:n==3)
 load_off(); results['rps_down']=wait_for('RPS fall',lambda n:n==1,210)
 policy.update(metric='cpu_utilization',targetValue=.2,version=p['version'])
 p=api('policies/'+p['id'],policy,'PUT')
 load_on(20000); results['cpu_up']=wait_for('CPU rise',lambda n:n==3,210)
 load_off(); results['cpu_down']=wait_for('CPU fall',lambda n:n==1,240)
 results['passed']=True
except Exception as e:
 results['error']=str(e); results['passed']=False
finally:
 if not stop.is_set():load_off()
 results.update(requests=count,requestErrors=errors)
 OUT.joinpath('load-results.json').write_text(json.dumps(results,indent=2))
 OUT.joinpath('operations.json').write_text(json.dumps(api('operations?pageSize=200'),indent=2,ensure_ascii=False))
 print(json.dumps(results),flush=True)

sys.exit(0 if results.get("passed") else 1)
