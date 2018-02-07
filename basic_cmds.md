Create:

```shell
docker network create --driver contrail-plugin:latest --subnet 78.66.11.0/24 --gateway 78.66.11.254 example_net3

docker service create --name example_bot3 --replicas 4 --network example_net3 dockersamples/examplevotingapp_vote:before

./contrailctl.sh -service-name example_bot3 -os-tenant-name admin -protocol-port 80 -protocol HTTP -algo ROUND_ROBIN -overwrite-vip 78.66.11.111 -floating-ip public-network:fip-pool-public:10.84.59.27

./contrailctl.sh -service-name example_bot3 -os-tenant-name admin -security-group block-http

./contrailctl.sh -service-name example_bot3 -os-tenant-name admin -delete-security-group block-http
```


Delete:

```shell
./contrailctl.sh -service-name example_bot3 -os-tenant-name admin -delete-security-group block-http

./contrailctl.sh -service-name example_bot3 -os-tenant-name admin -delete true

docker service rm example_bot3
```
