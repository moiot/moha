import subprocess
# az is short for "Available Zone"
# init az set
az_config = {
    "mysql-node-1": "az1",
    "mysql-node-2": "az2",
    "mysql-node-3": "az3",
    "etcd-node-0": "az1",
    "etcd-node-1": "az1",
    "etcd-node-2": "az2",
    "etcd-node-3": "az3",
    "etcd-node-4": "az3",
}


def ip_of(node):
    s = ["docker", "inspect", "-f", "'{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}'", node]
    ip = subprocess.check_output(s).decode("utf-8")
    return ip.replace("\n", "").replace("\'", "")


az_set = set([az_config[x] for x in az_config])
# az_dict is ip -> az mapping
az_dict = {}
for k in az_config:
    az_dict[ip_of(k)] = az_config[k]


az_node_mapping = {}
for k in az_set:
    az_node_mapping[k] = set()
for k in az_config:
    az_node_mapping[az_config[k]].add(k)

az_ip_mapping = {}
for k in az_set:
    az_ip_mapping[k] = set()
for k in az_dict:
    az_ip_mapping[az_dict[k]].add(k)

partition_template = "docker run -v /var/run/docker.sock:/var/run/docker.sock --rm -d pumba pumba netem -d 1m "


def run_query(query):
    print(query)
    subprocess.check_call(query.split())


def partition_outgoing(partitioned_az, partition_type):
    if partitioned_az not in az_set:
        print("{} is not one of the azs {}".format(partitioned_az, az_set))
        return
    query = str(partition_template)
    for ip in az_dict:
        if ip == "":
            continue
        if az_dict[ip] == partitioned_az:
            continue
        query += "--target {} ".format(ip)
    partitioned_nodes = az_node_mapping[partitioned_az]
    for node in partitioned_nodes:
        node_query = query + "{} {} ".format(partition_type, node)
        run_query(node_query)


def partition_incoming(partitioned_az, partition_type):
    if partitioned_az not in az_set:
        print("{} is not one of the azs {}".format(partitioned_az, az_set))
        return
    partitioned_nodes = az_ip_mapping[partitioned_az]
    print("the other direction")
    query = str(partition_template)
    for node in partitioned_nodes:
        query += "--target {} ".format(node)
    for n in az_config:
        if az_config[n] == partitioned_az:
            continue
        node_query = query + "{} {} ".format(partition_type, n)
        run_query(node_query)


def partition(partitioned_az):
    partition_type = "delay --time 3000 "
    partition_outgoing(partitioned_az, partition_type)
    partition_incoming(partitioned_az, partition_type)


def partition_each_other():
    partition_type = "loss -p 70 "
    for az in az_set:
        partition_outgoing(az, partition_type)


if __name__ == '__main__':
    # print(az_dict)
    # print(az_set)
    # print(az_ip_mapping)
    # partition("az1")
    # partition("az2")
    # partition("az3")
    # partition("az4")
    partition_each_other()

