# BinLead Webscoket Server
## Installation

> AWS EC2 2CPU/4RAM (t3.medium)


### Git clone
Git
```
git clone https://github.com/volkovartem77/binlead_wss.git
cd binlead_wss
```

### Set ENVIRONMENT VARIABLES
```
export REDIS_PASSWORD="^6D3or54g%7+Wze4{?TSC5sF"
export NATS_PASSWORD="sOJV1JRVeS3uN5rz5Z0S5Bmp"
export SECRET_KEY="af0660f986d713761085f8ded052f25f"
```

### Run the Docker container
```
sudo -E docker compose up --build -d
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