import requests
import redis
import time
import os
import platform
import threading

OUTSTANDING_REQUESTS_KEY = 'outstanding_requests'
print("Starting central controller updater")


class CentralControllerUpdater:
    def __init__(self, wait_interval_seconds, outstanding_request_key):
        cc_ip = '10.101.101.101'
        cc_port = 3000

        self.cc_url = f'http://{cc_ip}:{cc_port}/'
        print("Central controller url:", self.cc_url)
        self.redis = self.get_redis()
        print("Redis:", self.redis)

        self.wait_interval_seconds = wait_interval_seconds
        self.outstanding_request_key = outstanding_request_key

    @staticmethod
    def get_redis():
        port = 6379
        redis_ip = "localhost"

        node_name = os.environ.get('MY_NODE_NAME')
        if node_name == "minikube-m02":
            redis_ip = "10.101.102.101"
        elif node_name == "minikube-m03":
            redis_ip = "10.101.102.102"
        elif node_name == "minikube-m04":
            redis_ip = "10.101.102.103"

        rds = redis.Redis(host=redis_ip, port=port, db=0)

        print("Connected to redis at " + redis_ip + ":" + str(port))

        return rds

        # redis_ips = "10.101.102.101", "10.101.102.102", "127.0.0.1"
        # for redis_ip in redis_ips:
        #     rds = redis.Redis(host=redis_ip, port=port, db=0, socket_connect_timeout=1)
        #     try:
        #         rds.ping()
        #         print("Connected to redis at " + redis_ip + ":" + str(port))
        #         return rds
        #     except redis.exceptions.ConnectionError:
        #         print("Could not connect to redis at " + redis_ip + ":" + str(port))
        #     except redis.exceptions.TimeoutError:
        #         print("Redis connection timed out at " + redis_ip + ":" + str(port))
        #     except Exception:
        #         print("Unknown error connecting to redis at " + redis_ip + ":" + str(port))
        # raise Exception("Could not connect to redis")
    
    def do_continuous_update(self):
        current_time = time.time()
        current_time = (current_time * 1000) % 1000
        wait_time = 1000 - current_time

        time.sleep(wait_time / 1000)

        def repeat():
            threading.Timer(self.wait_interval_seconds, repeat).start()

            self.update()

        repeat()

    def update(self):
        outstanding_requests = self.get_outstanding_requests()
        print("outstanding requests:", outstanding_requests)

        try:
            requests.get(self.cc_url, headers={"Connection": "close"}, params={'podname': platform.node(), 'k': int(time.time()), 'a': outstanding_requests}, timeout=0.75)
        except requests.exceptions.Timeout:
            print("The request timed out")
        except requests.exceptions.RequestException as e:
            print("An error occurred:", e)

    def get_outstanding_requests(self):

        try:
            outstanding_request = self.redis.get(self.outstanding_request_key)
        except redis.exceptions.ConnectionError:
            print("Could not connect to redis for getting outstanding requests")
        except redis.exceptions.TimeoutError:
            print("Redis connection timed out for getting outstanding requests")
        except Exception:
            print("Unknown error connecting to redis for getting outstanding requests:", Exception)
        
        if outstanding_request is None:
            return 0
        else:
            return int(outstanding_request)


updater = CentralControllerUpdater(1, OUTSTANDING_REQUESTS_KEY)
updater.do_continuous_update() 