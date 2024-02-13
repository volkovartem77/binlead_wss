# BinLead Webscoket Server
## Installation

> AWS EC2 2CPU/4RAM (t3.medium)


### Git clone
Git
```
git clone https://github.com/volkovartem77/binlead_wss.git
cd binlead_wss
```

### Set Env
```
export REDIS_PASSWORD="^6D3or54g%7+Wze4{?TSC5sF"
export NATS_PASSWORD="sOJV1JRVeS3uN5rz5Z0S5Bmp"
```

### Install and configure NGINX
```
sudo docker run --name nginx-proxy --network mynetwork -d -p 80:80 -p 443:443 \
    -v /home/ubuntu/binlead_wss/nginx.conf:/etc/nginx/nginx.conf \
    -v /etc/letsencrypt:/etc/letsencrypt \
    nginx
```

### Build the Docker image
```
sudo docker build -t websocket-server .
```

### Run the Docker container
```
sudo docker run --network mynetwork -d -p 80:8080 -e REDIS_PASSWORD=yourRedisPassword -e NATS_PASSWORD=yourNatsPassword websocket-server
```

### How to make daily restart using Cron

#### 1. Edit the sudoers file
```
sudo visudo
```

#### 2. Add a NOPASSWD entry

```
ubuntu ALL=(ALL) NOPASSWD: /usr/bin/docker container restart 5a75150991d7
```

#### 3. Open cron
```
crontab -e
```

#### 4. Add the following line to schedule  
```
0 0 * * * TZ=UTC sudo /usr/bin/docker container restart 5a75150991d7
```