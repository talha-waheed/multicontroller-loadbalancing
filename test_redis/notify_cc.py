import os
import redis
from time import time, sleep

NOTIF_TIME_INTERVAL_MS = 1000

r = redis.Redis(host='localhost', port=6379, db=0)

while True:

    s = time()
    r.incr('foo')
    print(f"time for incr: {(time() - s)*1000:.5f}ms")

    s = time()
    r.decr('foo')
    print(f"time for decr: {(time() - s)*1000:.5f}ms")

    s = time()
    r.get('foo')
    print(f"time for get: {(time() - s)*1000:.5f}ms")

    sleep(0.1)


    os.getenv("CENTRAL_CONTROLLER_IP")