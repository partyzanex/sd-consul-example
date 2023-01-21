# Service Discovery example with Consul

### Usage:

Clone this repository:
```shell
git clone github.com/partyzanex/sd-consul-example.git && cd ./sd-consul-example
```

Running Consul in docker:
```shell
docker-compose up -d
```

Run in first terminal:
```shell
go run main.go --id=example-id-1 --port=8001
```

Run in second terminal:
```shell
go run main.go --id=example-id-2 --port=8002
```

Output:
```
2023/01/21 21:12:04 service example-id-1, status passing, address m1.local:8001
2023/01/21 21:12:04 service example-id-2, status passing, address m1.local:8002
2023/01/21 21:12:05 service example-id-1, status passing, address m1.local:8001
2023/01/21 21:12:05 service example-id-2, status passing, address m1.local:8002
...
```
