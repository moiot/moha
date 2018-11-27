#!/usr/bin/env python2

from __future__ import print_function, \
    unicode_literals

import urllib
import urllib2
import base64
import json
import argparse

parser = argparse.ArgumentParser(description='Process some integers.')
parser.add_argument('-f', dest="file", type=str,
                   help='the dashboard json file that need importing')

try:
    input = raw_input
except:
    pass

############################################################
################## CONFIGURATION ###########################
############################################################

dest = {"datasource": "Prometheus", "user":"admin", "password": "admin", "url":"http://127.0.0.1:3000/"}



############################################################
################## CONFIGURATION ENDS ######################
############################################################

def export_dashboard(api_url, api_key, dashboard_name):
    req = urllib2.Request(api_url + 'api/dashboards/db/' + dashboard_name,
                          headers={'Authorization': "Bearer {}".format(api_key)})

    resp = urllib2.urlopen(req)
    data = json.load(resp)
    return data['dashboard']


def fill_dashboard_with_dest_config(dashboard, dest):
    dashboard['id'] = None
    
    for panel in dashboard['panels']:
        panel['datasource'] = dest['datasource']

    if 'templating' in dashboard:
        for templating in dashboard['templating']['list']:
            if templating['type'] == 'query':
                templating['current'] = {}
                templating['options'] = []
            templating['datasource'] = dest['datasource']

    return dashboard

def import_dashboard(api_url, api_key, dashboard):
    payload = {'dashboard': dashboard,
               'overwrite': True}
    headers = {'Authorization': "Bearer {}".format(api_key),
               'Content-Type': 'application/json'}
    req = urllib2.Request(api_url + 'api/dashboards/db',
                          headers=headers,
                          data=json.dumps(payload))
    try:
        resp = urllib2.urlopen(req)
        data = json.load(resp)
        return data
    except urllib2.HTTPError, error:
        data = json.load(error)
        return data

def import_dashboard_via_user_pass(api_url, user, password, dashboard):
    payload = {'dashboard': dashboard,
               'overwrite': True}
    auth_string = base64.b64encode('%s:%s' % (user, password))
    headers = {'Authorization': "Basic {}".format(auth_string),
               'Content-Type': 'application/json'}
    req = urllib2.Request(api_url + 'api/dashboards/db',
                          headers=headers,
                          data=json.dumps(payload))
    try:
        resp = urllib2.urlopen(req)
        data = json.load(resp)
        return data
    except urllib2.URLError, error:
        return error.reason

if __name__ == '__main__':

    args = parser.parse_args()
    print ("importing ", args.file)
    dashboard = json.load(open(args.file))

    dashboard = fill_dashboard_with_dest_config(dashboard, dest)

    print("[import] <{}> to [{}]".format(
        dashboard['title'], dest['url']), end='\t............. ')
    if 'user' in dest:
        ret = import_dashboard_via_user_pass(dest['url'], dest['user'], dest['password'], dashboard)
    else:
        ret = import_dashboard(dest['url'], dest['key'], dashboard)

    if isinstance(ret,dict):
        if ret['status'] != 'success':
            print('ERROR: ', ret)
            raise RuntimeError
        else:
            print(ret['status'])
    else:
        print('ERROR: ', ret)
        raise RuntimeError
