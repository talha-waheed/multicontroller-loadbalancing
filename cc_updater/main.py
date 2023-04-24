import requests
import redis
import time
import os

OUTSTANDING_REQUESTS_KEY = 'outstanding_requests'


class CentralControllerUpdater:
    def __init__(self, wait_interval_seconds, outstanding_request_key):
        cc_ip = os.getenv('CENTRAL_CONTROLLER_IP') or 'localhost'
        cc_port = 3000

        self.cc_url = f'http://{cc_ip}:{cc_port}/'
        self.redis = redis.Redis(host='localhost', port=6379, db=0)

        self.wait_interval = wait_interval_seconds
        self.outstanding_request_key = outstanding_request_key

    
    def do_continuous_update(self):
        current_time = time.time()
        current_time = (current_time * 1000) % 1000
        wait_time = 1000 - current_time

        time.sleep(wait_time / 1000)

        while True:
            self.update()
            time.sleep(self.wait_interval_seconds)

    def update(self):
        outstanding_requests = self.get_outstanding_requests()

        requests.get(self.cc_url, params={'outstanding_requests': outstanding_requests})


    def get_outstanding_requests(self):

        outstanding_request = self.redis.get(self.outstanding_request_key)

        if outstanding_request is None:
            return 0
        else:
            return int(outstanding_request)


updater = CentralControllerUpdater(1, OUTSTANDING_REQUESTS_KEY)
updater.do_continuous_update()