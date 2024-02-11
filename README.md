# BinLead Webscoket Server
## Installation

> AWS EC2 2CPU/4RAM (t3.medium)

### Git clone
Git
```
git clone https://github.com/volkovartem77/binlead_wss.git
```

### Build the Docker image
```
docker build -t websocket-server .
```

### Run the Docker container
```
docker run --network mynetwork -d -p 80:8080 -e REDIS_PASSWORD=yourRedisPassword -e NATS_PASSWORD=yourNatsPassword websocket-server
```