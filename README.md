# BinLead Webscoket Server
## Installation

> AWS EC2 2CPU/4RAM (t3.medium)

### Git clone
Git
```
git clone https://github.com/volkovartem77/binlead_wss.git
cd binlead_wss
```

### Build the Docker image
```
sudo docker build -t websocket-server .
```

### Run the Docker container
```
sudo docker run --network mynetwork -d -p 80:8080 -e REDIS_PASSWORD=yourRedisPassword -e NATS_PASSWORD=yourNatsPassword websocket-server
```