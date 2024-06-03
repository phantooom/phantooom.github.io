&---
title: HomeLab(二) 黑群晖及常用软件配置
categories: [HomeLab]
date: 2024-6-3
keywords: ['群晖','immich','备份','nas','追番','自动化','jellyfin','影视']
tags:
    - 群晖
    - immich
    - 备份
    - nas
    - 追番
    - 自动化
    - jellyfin
    - 影视
---

# 背景

之前从8100的黑群晖，切换到了N100的，但是比较慢，然后忍不了了，就又新买了一套设备组装。趁机就升级下网卡。之所以选黑群晖就是只是因为界面友好点。配套软件比较给力。然后穷买不起白裙。
<!-- more -->
## 需求

- 存储
    - 文件存储(无需磁盘级别高可用) 群晖自带
    - 照片存储 immich
    - 提供远程块设备 群晖自带
    - webdav 群晖自带
    - 远端数据备份 群晖自带（只备份照片，其他的内容如果丢了就让他随风而逝吧）
- 影音
    - 下载 qbittorrent ipv6 + 不期望再走一层群晖的nat
    - 自动追番 (autobangumi)
    - 电影下载&封面刮削 (moviepilot)

## 硬件配置

整体都是走的捡垃圾的路线，能省就剩

CPU: 12300t

主板: 尔英B660m

内存: 十铨冥神 ddr4 16g * 2 

电源: 振华 80 plus 金牌 450w

网卡: mcx4121(中兴oem) 10G 双口，之前用的是一个 MNPA19- XTR 死活驱不上，就换了mcx4121

硬盘: 

- 10T * 1
- 12T * 1
- 14T * 2
- 16T * 1

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled.png)

sata 线比较乱,感觉换成定制线会好点。后边有预算了再看吧。

# 环境配置

## 系统

引导走的RR，机型选的SA6400，主要是为了能够支持12代intel的核显。

## 存储池

所有的存储池都是basic，后边有问题恢复也相对容易，不要做各种级别的raid。

## 证书

下载acmesh

```bash
wget https://github.com/acmesh-official/acme.sh/archive/master.tar.gz
tar xvf master.tar.gz
cd acme.sh-master/
./acme.sh --install --nocron --home /usr/local/share/acme.sh --accountemail "xiaorui.zou@gmail.com"
source ~/.profile
```

签发证书

```bash
export CF_Token="你的TOKEN"
export CF_Email="xiaorui.zou@gmail.com"
cd /usr/local/share/acme.sh
export CERT_DOMAIN="*.home.zou.cool"
export CERT_DNS="dns_cf"
./acme.sh --issue --server letsencrypt --home . -d "$CERT_DOMAIN" --dns "$CERT_DNS"
```

正确的话会返回

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%201.png)

替换群晖的根证书

```bash
cd /usr/local/share/acme.sh
export SYNO_Username='phantooom'
export SYNO_Password='你的密码'
export SYNO_Certificate="bp"
./acme.sh --deploy --home . -d "$CERT_DOMAIN" --deploy-hook synology_dsm
```

成功会返回如下内容

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%202.png)

我们登录群晖会发现证书已经被替换了

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%203.png)

证书自动续签我们添加一个任务，使用root执行

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%204.png)

### 使用域名登录

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%205.png)

去cloudflare增加一个泛解析即可。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%206.png)

## 内置软件安装

1. container Manager （docker）
2. webstation
3. cloud sync

## 第三方软件

### 下载软件

这里使用的是qbittorrent ，我们需要支持ipv6，也需要一个与宿主机在同一网段的IP。所以我们需要新建一个macvlan的设备用来支持这种场景。

我们给这个macvlan设备分配一个网段。

192.168.50.128/28（192.168.50.129~142）总共10多个ip目前看足够用了。（分的时候多分点，这个设备修改subnet非常费事。）

创建macvlan

```bash
docker network create -d macvlan \
--subnet=192.168.50.1/24 \
--gateway=192.168.50.1 \
--ipv6 \
--subnet=fd00::/64 \
-o parent=eth1 \
bridge-host
```

打通宿主机与macvlan，如果不打通后期没有办法使用群辉的webstation统一管理前边的接入层，记得加入到定时任务

```bash
ip link add macvlan-host link eth1 type macvlan mode bridge
ip addr add 192.168.50.128 dev macvlan-host
ip link set macvlan-host up
ip route add 192.168.50.128/28 dev macvlan-host
```

配置如下

```bash
version: "3.5"
services:
  qbittorrent:
    container_name: qBittorrent
    environment:
      - TZ=Asia/Shanghai
      - WEBUI_PORT=8080
      - PUID=1026
      - PGID=100
    volumes:
      - /volume3/homelab/docker/qb:/config
      - /volume3/downloads:/downloads # 填入下载绝对路径
      - /volume3/影音:/影音
    ports:
      - "8080:8080"
      - "6881:6881"
      - "6881:6881/udp"
    networks:
      default:
        ipv4_address: 192.168.50.129
    restart: always
    image: superng6/qbittorrent
networks:
  default:
    external: true
    name: bridge-host
```

配置一下反向代理，这个不能直接使用webstation接入，主要是因为使用了macvlan的方式。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%207.png)

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%208.png)

我们可以看到ipv4，v6都有链接。

配置一下内网跳过安全验证

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%209.png)

### 影音库

这边使用的是jellyfin

```bash
version: '3.5'
services:
  jellyfin:
    image: jellyfin/jellyfin
    container_name: jellyfin
    ports:
    - 18096:8096
    volumes:
      - /volume3/homelab/docker/jellyfin:/config
      - /volume3/homelab/docker/jellyfin/cache
      - /volume3/影音:/影音
    restart: always
```

在webstation中配置一下域名

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2010.png)

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2011.png)

记得申请一个apikey

### 自动化追番软件

自动追番我们采用 autobangumi + [mikanani.me](http://mikanani.me/) 从mikan上面获取自己想要的动画的rss，后续更新了就会自动下载。并且改名。

配置如下:

```bash
version: "3.5"
services:
  auto_bangumi:
    container_name: AutoBangumi
    environment:
      - TZ=Asia/Shanghai
      - PGID=${GID}
      - PUID=${UID}
    volumes:
      - /volume3/homelab/docker/auto-bangumi:/app/config
      - /volume3/homelab/docker/auto-bangumi:/app/data
      - /volume3/downloads:/downloads # 填入下载绝对路径
      - /volume3/影音:/影音
    ports:
      - '17892:7892'
    dns:
      - 8.8.8.8
      - 223.5.5.5
    restart: always
```

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2012.png)

具体配置如下，可以打开电报的通知。当然也能微信通知，自己配置一下就好了。

记得设置下载器地址。就是我们之前搭的qb因为我们配置了局域网面密，所以这个密码啥的都不用管。这个代理可以完全不用配置，如果在路由上面做了透明代理。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2013.png)

通知大体如下

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2014.png)

大体上只要把这个季度的想看的新番填上就能自动下载了。基本上回来看到更新了就能开始看了。

### 电影&电视剧下载

一般电影类的可能习惯使用bt/网盘下载。电视剧类的更习惯走pt，但是不管哪种方式最后都要用moviePilot去整理&获取封面资源&字幕。

```bash
version: "3.5"
services:
  nas-tools:
    image: jxxghp/moviepilot:latest
    ports:
      - 13000:3000        # 默认的webui控制端口
    volumes:
      - /volume3/homelab/docker/moviepilot:/config
      - /volume3/homelab//docker/moviepilot:/moviepilot
      - /volume3/影音:/影音
      - /volume3/downloads:/download
    environment:
      - PUID=0    # 想切换为哪个用户来运行程序，该用户的uid
      - PGID=0    # 想切换为哪个用户来运行程序，该用户的gid
      - UMASK=000 # 掩码权限，默认000，可以考虑设置为022

    restart: always
    hostname: moviepilot
    container_name: moviepilot
```

分类配置

```bash
####### 配置说明 #######
# 1. 该配置文件用于配置电影和电视剧的分类策略，配置后程序会按照配置的分类策略名称进行分类，配置文件采用yaml格式，需要严格附合语法规则
# 2. 配置文件中的一级分类名称：`movie`、`tv` 为固定名称不可修改，二级名称同时也是目录名称，会按先后顺序匹配，匹配后程序会按这个名称建立二级目录
# 3. 支持的分类条件：
#   `original_language` 语种，具体含义参考下方字典
#   `production_countries` 国家或地区（电影）、`origin_country` 国家或地区（电视剧），具体含义参考下方字典
#   `genre_ids` 内容类型，具体含义参考下方字典
#   themoviedb 详情API返回的其它一级字段
# 4. 配置多项条件时需要同时满足，一个条件需要匹配多个值是使用`,`分隔

# 配置电影的分类策略
movie:
  # 分类名同时也是目录名
  动画电影:
    # 匹配 genre_ids 内容类型，16是动漫
    genre_ids: '16'
  华语电影:
    # 匹配语种
    original_language: 'zh,cn,bo,za'
  # 未匹配以上条件时，分类为外语电影
  外语电影:

# 配置电视剧的分类策略
tv:
  动画:
    # 匹配 genre_ids 内容类型，16是动漫
    genre_ids: '16'
  纪录片:
     # 匹配 genre_ids 内容类型，99是纪录片
    genre_ids: '99'
  儿童:
    # 匹配 genre_ids 内容类型，10762是儿童
    genre_ids: '10762'
  综艺:
    # 匹配 genre_ids 内容类型，10764 10767都是综艺
    genre_ids: '10764,10767'
  国产剧:
    # 匹配 origin_country 国家，CN是中国大陆，TW是中国台湾，HK是中国香港
    origin_country: 'CN,TW,HK'
  欧美剧:
    # 匹配 origin_country 国家，主要欧美国家列表
    origin_country: 'US,FR,GB,DE,ES,IT,NL,PT,RU,UK'
  日韩剧:
    # 匹配 origin_country 国家，主要亚洲国家列表
    origin_country: 'JP,KP,KR,TH,IN,SG'
  # 未匹配以上分类，则命名为未分类
  未分类:
```

![微信图片_20240603221357.jpg](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E5%259B%25BE%25E7%2589%2587_20240603221357.jpg)

安装好了如上图所示。我们进行一些基本的配置

![微信截图_20240603221605.jpg](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E6%2588%25AA%25E5%259B%25BE_20240603221605.jpg)

启用本地的cookiecloud。将我们的地址填入cookiecloud的浏览器插件中。后边我们要自动同步许多支持搜索的站点用来搜索种子资源。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2015.png)

填写完成之后我们去点击服务选项卡中的同步cookiecloud站点

![微信截图_20240603221923.jpg](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E6%2588%25AA%25E5%259B%25BE_20240603221923.jpg)

此时我们在站点管理里面就能看到我们的站点了，馒头的支持比较复杂参考 这个文章[https://t.me/moviepilot_official/507610](https://t.me/moviepilot_official/507610)

![微信截图_20240603222034.jpg](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E6%2588%25AA%25E5%259B%25BE_20240603222034.jpg)

我们点击设定，选择搜索从哪些站点搜索

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2016.png)

目录我们根据自己实际情况设定好。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2017.png)

如果这里你下载是pt，同时目录又跨盘了，尽量选择复制的方式，然后后边通过其他插件去自动清理下载目录。

下载相关的配置，配置好即可。这个这个端口可千万别忘了加，坑了半天，真的不能没有端口！人家确实没骗我，是我草率了。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2018.png)

我们去搜搜一下资源试试吧

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2019.png)

比如我们想体验一下，背后有一个强大的祖国。想了解什么是犯我中华者虽远必诛。那么我们就看一部战狼2吧。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2020.png)

点击确认即可

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2021.png)

这边会开始下载，下载完了会有通知

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2022.png)

![微信截图_20240603235746.jpg](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E6%2588%25AA%25E5%259B%25BE_20240603235746.jpg)

下载好了，我就不看了，毕竟都快背下来了。

还有一些比较有意思的插件简单的提一下

站点自动签到，就是一些pt站是要求登录的，但是我们平时可能也不怎么去，这个就会模拟迁到或者登录，保证账号不会被删除。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2023.png)

豆瓣想看，你在豆瓣标记好自己相看的，就会自己去下载了。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2024.png)

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2025.png)

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2026.png)

自动删种，pt一般都上传有要求的，所以不能下完就删除，所以要等一段时间，我们可以设定一个分享率或者时间去自动删除下载任务。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2027.png)

### 照片管理

最开始我是用photos的大体上用了2年多，非常好用。后边出了immich就体验了一下，也挺好用的，所以。本篇文章切换到了immich来讲解

```bash
name: immich

services:
  immich-server:
    container_name: immich_server
    image: ghcr.io/immich-app/immich-server:${IMMICH_VERSION:-release}
    command: ['start.sh', 'immich']
    volumes:
          - ${UPLOAD_LOCATION}:/usr/src/app/upload
          - ${EXTERNAL_PATH}:/usr/src/app/external
          - /etc/localtime:/etc/localtime:ro
    env_file:
      - .env
    ports:
      - 2283:3001
    depends_on:
      - redis
      - database
    restart: always

  immich-microservices:
    container_name: immich_microservices
    image: ghcr.io/immich-app/immich-server:${IMMICH_VERSION:-release}
    # extends: # uncomment this section for hardware acceleration - see https://immich.app/docs/features/hardware-transcoding
    #   file: hwaccel.transcoding.yml
    #   service: cpu # set to one of [nvenc, quicksync, rkmpp, vaapi, vaapi-wsl] for accelerated transcoding
    command: ['start.sh', 'microservices']
    volumes:
      - ${UPLOAD_LOCATION}:/usr/src/app/upload
      - /etc/localtime:/etc/localtime:ro
      - ${EXTERNAL_PATH}:/usr/src/app/external
    env_file:
      - .env
    depends_on:
      - redis
      - database
    restart: always

  immich-machine-learning:
    container_name: immich_machine_learning
    # For hardware acceleration, add one of -[armnn, cuda, openvino] to the image tag.
    # Example tag: ${IMMICH_VERSION:-release}-cuda
    image: ghcr.io/immich-app/immich-machine-learning:${IMMICH_VERSION:-release}
    # extends: # uncomment this section for hardware acceleration - see https://immich.app/docs/features/ml-hardware-acceleration
    #   file: hwaccel.ml.yml
    #   service: cpu # set to one of [armnn, cuda, openvino, openvino-wsl] for accelerated inference - use the `-wsl` version for WSL2 where applicable
    volumes:
      - model-cache:/cache
    env_file:
      - .env
    restart: always

  redis:
    container_name: immich_redis
    image: registry.hub.docker.com/library/redis:6.2-alpine@sha256:84882e87b54734154586e5f8abd4dce69fe7311315e2fc6d67c29614c8de2672
    restart: always

  database:
    container_name: immich_postgres
    image: registry.hub.docker.com/tensorchord/pgvecto-rs:pg14-v0.2.0@sha256:90724186f0a3517cf6914295b5ab410db9ce23190a2d9d0b9dd6463e3fa298f0
    environment:
      POSTGRES_PASSWORD: ${DB_PASSWORD}
      POSTGRES_USER: ${DB_USERNAME}
      POSTGRES_DB: ${DB_DATABASE_NAME}
      POSTGRES_INITDB_ARGS: '--data-checksums'
    volumes:
      - ${DB_DATA_LOCATION}:/var/lib/postgresql/data
    restart: always
    command: ["postgres", "-c" ,"shared_preload_libraries=vectors.so", "-c", 'search_path="$$user", public, vectors', "-c", "logging_collector=on", "-c", "max_wal_size=2GB", "-c", "shared_buffers=512MB", "-c", "wal_compression=on"]
volumes:
  model-cache:
```

```bash
UPLOAD_LOCATION=/volume4/immich-data/photos
DB_DATA_LOCATION=/volume4/immich-data/postgres
EXTERNAL_PATH=/volume3/photos
# The Immich version to use. You can pin this to a specific version like "v1.71.0"
IMMICH_VERSION=release

# Connection secret for postgres. You should change it to a random password
DB_PASSWORD=postgres

# The values below this line do not need to be changed
###################################################################################
DB_USERNAME=postgres
DB_DATABASE_NAME=immich
```

EXTERNAL_PATH 是外部的路径，可以加载之前整理好的照片。一定要注意，如果你以前照片是被群晖管理的可能存在许多叫做 @eaDir 的文件夹，这里面都是缩略图缓存，一定要全都删除掉。

```bash
find /volume* -name "@eaDir" -type d -print0 | xargs -0 rm -rf
```

不然缩略图也会被当成照片再解析一次，非常酸爽。

安装好如下图所示。

![微信截图_20240603232542.jpg](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E6%2588%25AA%25E5%259B%25BE_20240603232542.jpg)

我们调整下设置，模型换成 XLM-Roberta-Large-Vit-B-16Plus 这个支持中文，记得要在添加照片之前就换好。不然要重新提取才可以搜索。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2028.png)

支持人脸聚类，支持地点展示

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2029.png)

如果地图带有GPS信息则也支持展示到地图中

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2030.png)

也支持内容搜索，至于说效果也就一般般，可能跟手机内置的相册差不多水平吧。

![微信截图_20240603233948.png](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/%25E5%25BE%25AE%25E4%25BF%25A1%25E6%2588%25AA%25E5%259B%25BE_20240603233948.png)

APP支持同步手机相册到远端

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled.jpeg)

## 数据备份

数据备份使用cloud sync

这边只备份照片数据，其他的均不备份。

一份备份到自己的另一台nas，一份备份到百度云盘

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2031.png)

有两个小点要注意的是。如下三点:

- 同步方向要仅本地上传的变更。这个是防止云上误操作，或者盗号导致同步到本地，本地数据也没了
- 数据加密，百度云比较恶心人，如果他认为你的照片有问题，那么这个照片就会被删掉。而你是没有资格保存这张照片的。可能你的青春就都没了。再也找不回来了。
- 当删除源文件夹文件时，不要删除目的地文件夹中的文件。这个也是血泪教训，如果手残删了本地，没有勾选这个的话，远程也会被删掉。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-synology/Untitled%2032.png)

# 其他

- 虽然升级了万兆网卡，但是实际上读不分散在几块盘，读不分散在多个机器实际上是利用不上这个东西的。所以可能后边会考虑加一个读缓存。
- 整体上moviepilot能满足大部分的影音需求，但是在动画方面还差一些。主要是针对追番场景下的订阅不太友好。完全是基于搜索实现的。我更想明确的跟某个字幕组的翻译，而不是随机来。
- immich 还是挺有意思的。
- 共享文件夹没有提，但是其实这个才是日常打交道最多的。
- 带颜色的东西也有做，但是不太好分享，但是也挺有意思的。
- 没有在群晖搭homeassist，这个东西我放到路由里面了，因为家电主要在上边。另外就是不少家电都没有接口。
- 整体从性能看12300t 比N100 舒服不少了。至少体验上来说是这样。但是跟8100比，我目前的场景没觉得太大的提升。
- 数据安全始终要放到第一位，切记。切记。