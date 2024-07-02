# raft-example
This is a simple raft demo develop up on [hashicorp/raft](hashicorp/raft),

### Usage

#### Start cluster

1. Start 1st raft node:

   ```sh
   $ raft --http-address 0.0.0.0:5000 --raft-address 192.168.1.42:6000 --rpc-address=0.0.0.0:7000 --serf-address 192.168.1.42:8000 --data-dir /node0 --bootstrap
   ```

2. Start 2st raft node:

   ```sh
   $ raft --http-address 0.0.0.0:5001 --raft-address 192.168.1.42:6001 --rpc-address=0.0.0.0:7001 --serf-address 192.168.1.42:8001 --data-dir /node1 --joinAddress 192.168.1.42:8000
   ```

3. Start 3st raft node:

   ```sh
   $ raft --http-address 0.0.0.0:5002 --raft-address 192.168.1.42:6002 --rpc-address=0.0.0.0:7002 --serf-address 192.168.1.42:8002 --data-dir /node2 --joinAddress 192.168.1.42:8000
   ```

> `--raft-address`can't be `0.0.0.0`, or there will throw an error *"local bind address is not advertisable"*, and `--serf-address` shouldn't be `0.0.0.0`

#### Write && Read

Write data to a raft node:

```sh
$ curl -XPOST localhost:5000/api/v1/set --header 'Content-Type: application/json' -d '
{
    "key" : "testKey",
    "value" : "testValue"
}'
```

Then you can get the data from any raft node:

```sh
$ curl -XGET localhost:5000/api/v1/get --header 'Content-Type: application/json' -d '
{
    "key" : "testKey"
}'
```

```shell
$ curl -XGET localhost:5001/api/v1/get --header 'Content-Type: application/json' -d '
{
    "key" : "testKey"
}'
```

```sh
$ curl -XGET localhost:5002/api/v1/get --header 'Content-Type: application/json' -d '
{
    "key" : "testKey"
}'
```

#### Get raft stat

You can use `/opera/stats` to get the raft state:

```sh
$ curl 0.0.0.0:5002/opera/stats|jq               
{
  "applied_index": "5",
  "commit_index": "5",
  "fsm_pending": "0",
  "last_contact": "62.214834ms",
  "last_log_index": "5",
  "last_log_term": "2",
  "last_snapshot_index": "0",
  "last_snapshot_term": "0",
  "latest_configuration": "[{Suffrage:Voter ID:0.0.0.0:7000 Address:10.21.6.13:6000} {Suffrage:Voter ID:0.0.0.0:7001 Address:10.21.6.13:6001} {Suffrage:Voter ID:0.0.0.0:7002 Address:10.21.6.13:6002}]",
  "latest_configuration_index": "0",
  "num_peers": "2",
  "protocol_version": "3",
  "protocol_version_max": "3",
  "protocol_version_min": "0",
  "snapshot_version_max": "1",
  "snapshot_version_min": "0",
  "state": "Follower",
  "term": "2"
}
```

#### Switch raft leader

You can stop the raft leader, then the leader role will shift to another raft node.

#### Generate .pb.go

```go
$ protoc --go_out=. --go_opt=paths=source_relative  --go-grpc_out=. --go-grpc_opt=paths=source_relative ./forward.proto 
```

More infor refer to this [article](https://www.cnblogs.com/charlieroro/p/17486646.html).
