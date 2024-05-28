---
title: HomeLab(一): 基础网络搭建
categories: [HomeLab]
date: 2024-5-28
keywords: ['网络','软路由','HomeLab','openvpn','openclash','tailscale']
tags:
    - 网络
    - 软路由
    - HomeLab
    - openvpn
    - openclash
    - tailscale
---

# 背景

之前在生产队当驴的时候，觉得生产队拉的磨盘，性能太差，拉的不太顺手。斥巨资购入了一套私磨拉。最近生产队买了更高性能的磨，于是我又换回公磨了。把私磨带回家里了，家里的设备变得就非常多。然后之前为了省电费把我的 intel 8100 的 NAS降级成了 intel N100。跑的容器越来越多。性能也跟不太上了。最后想了一下也别回intel 8100了，打算把nas升级一波，借着机会就把 HomeLab 搞起来。

<!-- more -->

# 房间概况

房间整体比较小，但是墙还挺多。一台Wifi可能不太够，同时在房间的楼下还有一间仓储。家住7楼，仓储在负一楼。之间的网络不太可能用双绞线，大体上要使用光纤连接。

# 网络设计

总体期望楼上是能够完全不依赖楼下去使用网络。楼下则是尽可能的不依赖楼上，但是因为出口在楼上所以总体上还是有很大依赖的。

## 公网出口

公网出口是一条电信的500M的宽带。因为千兆的太贵了。500M的+手机卡只要49一个月。所以就用了500M的宽带。

- 改了桥接使用软路由进行拨号
- 申请了公网的IPV4 IP
- 从光猫换成了猫棒（不是为了提高网速，只是为了节约弱电箱的空间）

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled.png)

## 内网规划

内网整体上分为两个大的部分一部分是楼上，一部分是楼下。

### 设备情况

楼上的设备主要是

- 笔记本
- 两台PC
- N台手机
- 一台威联通NAS
- 众多的智能家居

楼下的设备主要是

- 黑群晖
- N100主机日常上网使用
- 测试集群
    - AMD 5800 mini主机
    - Intel 1600X PC
    - intel 8100 PC

### 需求

- 期望群晖拥有万兆的出口
- 群晖上的容器应该分配到基础网络的IP，同时能够获取到IPV6的IP
- 楼上设备的IP可以是自动分配的，楼下的设备会承担一定的角色，所以要使用固定IP。
- 楼上与仓储之间拥有万兆的链路
- 无线网络单个AP应该提供2.5G的带宽

### 拓扑图

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%201.png)

### IP规划

192.168.50.1/24 dhcp仅分配前一半的IP后一半的IP手动分配，后一半手动分配的，主要是仓储的设备与群晖上跑的容器。

192.168.51.1/24 k8s集群网络

# 施工

## 弱电箱

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled.jpeg)

猫棒还是挺热的，可能夏天的时候还要加一个风扇。

## 楼上到仓储光纤

我跟我的老父亲安排了一波。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%201.jpeg)

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%202.jpeg)

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%203.jpeg)

熔纤机太贵了只能冷接。

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%204.jpeg)

# 扩展网络需求

所有的实现均落在软路由上

## 远程接入

可能需要同步照片，或者查看家中的一些资料。出于安全考虑不会将内网的东西暴露出去。需要接入内网访问。

### tailscale

用作主链路。远端无IPV4开启NAT1的情况下打洞成功率较高。远端有IPV6 IP 无需任何设置就能直连回来，不需要中转。

ipv6的情况

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%202.png)

ipv4的情况

![Untitled](https://cdn.jsdelivr.net/gh/phantooom/image-box/homelab-network/Untitled%203.png)

### openvpn

用作备用链路，配合ddns接入。只在连续打洞失败的情况下使用。

## 梯子

使用openclash做了透明代理，一些不存在的网站可以直接访问，特别是docker pull 啥的特别舒服，什么都不用配置。

# 其他

大体上简单梳理了一下这个当前的网络状态，后续会继续写关于pxe自动装机，黑群晖，跟k3s集群搭建，vm管理的相关内容。