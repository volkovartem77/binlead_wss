# BinLead Webscoket Server
## Installation

> AWS EC2 2CPU/4RAM (t3.medium)   

```
sudo su
cd
```

### Install Docker, Redis, NATS, Git

Docker
```
sudo apt install apt-transport-https ca-certificates curl software-properties-common
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt update
apt-cache policy docker-ce
sudo apt install docker-ce
sudo systemctl status docker
```
Redis
```
apt-get install redis-server -y
chown redis:redis /var/lib/redis
```
NATS
```
sudo docker run -d --restart unless-stopped -p 4222:4222 -p 8222:8222 -p 6222:6222 --name nats-server -ti nats:latest
```  
Git
```
apt-get install git-core -y
git clone https://github.com/volkovartem77/binlead_wss.git
```

### Build the Docker image
```
docker build -t websocket-server .
```

### Run the Docker container
```
docker run -d -p 80:8080 websocket-server
```