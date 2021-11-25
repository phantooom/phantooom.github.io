---
title: Nginx-Ingress-Controller 代码走读
date: 2021-11-24
categories: [kubernetes,ingress]
keywords: [kubernetes,ingress,网络]
tags:
    - kubernetes
    - ingress
    - 网络
---

<a name="t3Nbo"></a>
# 简介
nginx-ingress-controller 是最常用的 ingress-controller 之一，也是当前公司生产在使用的ingress。这边会分析主流程。整个ingress-controller是怎么工作的。并不会详细的去解释所有的代码。找到关键节点即可。<br />

<!-- more -->

<a name="ZJflU"></a>
# 整体流程
![nginx-ingress流程.png](https://cdn.jsdelivr.net/gh/phantooom/image-box/ingress/nginx-ingress-read/01.png)<br />

<a name="cBg2r"></a>
# 代码实现
<a name="VUkQm"></a>
## go部分
<a name="AWKCb"></a>
### 入口函数
通过构建脚本我们可以发现入口是/cmd/nginx
```go
// build/build.sh
go build \
  -trimpath -ldflags="-buildid= -w -s \
    -X ${PKG}/version.RELEASE=${TAG} \
    -X ${PKG}/version.COMMIT=${COMMIT_SHA} \
    -X ${PKG}/version.REPO=${REPO_INFO}" \
  -o "rootfs/bin/${ARCH}/nginx-ingress-controller" "${PKG}/cmd/nginx"
```
<a name="FV3Ej"></a>
### 初始化
初始化的过程中会初始化  NGINXController  结构如下
```go
// internal/ingress/controller/nginx.go
type NGINXController struct {
    // 这个结构体主要是一些配置相关的，如tcp/udp配置的map， apiserver地址，同步时间监听端口等
	cfg *Configuration
	// recoder是用来记录一些事件的比如reload等
	recorder record.EventRecorder
	// 一个work queue 用来同步状态比如ingress的资源的信息会被放到这里
	syncQueue *task.Queue
	// 用来同步ingress的状态的
	syncStatus status.Syncer
	// 用来限流的
	syncRateLimiter flowcontrol.RateLimiter

	// stopLock is used to enforce that only a single call to Stop send at
	// a given time. We allow stopping through an HTTP endpoint and
	// allowing concurrent stoppers leads to stack traces.
	stopLock *sync.Mutex
	// 停止的channel 用来传输stop的信息，实现优雅的关闭
	stopCh   chan struct{}
    // k8s store有更新的时候这个会收到消息
	updateCh *channels.RingChannel

	// ngxErrCh is used to detect errors with the NGINX processes
	ngxErrCh chan error
    
	// runningConfig contains the running configuration in the Backend
	runningConfig *ingress.Configuration
	// 存储 nginx的模板
	t ngx_template.Writer
	// 域名解析的 dns服务器
	resolver []net.IP
	// 是否开启ipv6
	isIPV6Enabled bool
	// 是否处于关闭中的状态
	isShuttingDown bool
	// tcp proxy的表
	Proxy *TCPProxy
	// 存储ingress信息的store
	store store.Storer
	// 指标记录使用
	metricCollector metric.Collector
	// webhook server的信息
	validationWebhookServer *http.Server
	// 执行命令的包装
	command NginxExecTester
}
```
接下来初始化store这个主要是初始化了各种lister,infromer以及eventhandler
```c
// internal/ingress/controller/store/store.go
// 初始化不同版本的ingress ep secret cm svc等
	store.informers.Ingress = infFactory.Networking().V1beta1().Ingresses().Informer()
	store.listers.Ingress.Store = store.informers.Ingress.GetStore()

	store.informers.Endpoint = infFactory.Core().V1().Endpoints().Informer()
	store.listers.Endpoint.Store = store.informers.Endpoint.GetStore()

	store.informers.Secret = infFactorySecrets.Core().V1().Secrets().Informer()
	store.listers.Secret.Store = store.informers.Secret.GetStore()

	store.informers.ConfigMap = infFactoryConfigmaps.Core().V1().ConfigMaps().Informer()
	store.listers.ConfigMap.Store = store.informers.ConfigMap.GetStore()

	store.informers.Service = infFactory.Core().V1().Services().Informer()
	store.listers.Service.Store = store.informers.Service.GetStore()
// 初始化不同的回调函数

	store.informers.Ingress.AddEventHandler(ingEventHandler)
	store.informers.Endpoint.AddEventHandler(epEventHandler)
	store.informers.Secret.AddEventHandler(secrEventHandler)
	store.informers.ConfigMap.AddEventHandler(cmEventHandler)
	store.informers.Service.AddEventHandler(serviceHandler)
```
接下来初始化taskQueue,这个Queue里面的任务会被消费，每次消费都会全量的刷新ingress的配置。
```c
//internal/ingress/controller/nginx.go
// 注意关注下这个syncIngress的func后边会详细介绍
n.syncQueue = task.NewTaskQueue(n.syncIngress)
```
初始化nginx-tmpl的配置，然后注册nginx-tmpl变更的回调，变更后会重新渲染配置然后reload nginx
```c
//internal/ingress/controller/nginx.go
// 删除了部分不相关代码
onTemplateChange := func() {
		template, err := ngx_template.NewTemplate(nginx.TemplatePath)
		n.t = template
		n.syncQueue.EnqueueTask(task.GetDummyObject("template-change"))
}
ngxTpl, err := ngx_template.NewTemplate(nginx.TemplatePath)
n.t = ngxTpl
_, err = watch.NewFileWatcher(nginx.TemplatePath, onTemplateChange)

```
启动controller
```c
//internal/ingress/controller/nginx.go
func (n *NGINXController) Start() {
    // 启动informer
	n.store.Run(n.stopCh)
// 选举
	setupLeaderElection(&leaderElectionConfig{
		Client:     n.cfg.Client,
		ElectionID: electionID,
        // 只有leader才会执行
		OnStartedLeading: func(stopCh chan struct{}) {
			if n.syncStatus != nil {
				go n.syncStatus.Run(stopCh)
			}
			
			n.metricCollector.OnStartedLeading(electionID)
			// manually update SSL expiration metrics
			// (to not wait for a reload)
			n.metricCollector.SetSSLExpireTime(n.runningConfig.Servers)
		},
        // 都会执行选举结束就会执行
		OnStoppedLeading: func() {
			n.metricCollector.OnStoppedLeading(electionID)
		},
	})
	// 启动nginx的命令
	cmd := n.command.ExecCommand()

	// 启动配置 PGID为0,表示新建一个子进程，防止NGINX进程收到controller的信号
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
		Pgid:    0,
	}
	启动nginx
	n.start(cmd)
	// 启动worker，从worker中不断获取任务会经过之前提到的 n.syncIngress 进行处理后面会解释
	go n.syncQueue.Run(time.Second, n.stopCh)

	for {
		select {
        // 获取异常信息，不重要略过
		case err := <-n.ngxErrCh:

		case event := <-n.updateCh.Out():
			if evt, ok := event.(store.Event); ok {
                // 将n.updateCh中获取的event放入工作队列中
				n.syncQueue.EnqueueSkippableTask(evt.Obj)
			} else {
				klog.Warningf("Unexpected event type received %T", event)
			}
        // 获取stop的信息不重要略过
		case <-n.stopCh:
			return
		}
	}
}
```
<a name="nnY16"></a>
### deltafifo消费
启动informer之后，注册到informer的eventhandler中的回调函数，会在deltafifo消费的过程中被执行。<br />也就是说意味着有 ingress,ep,cm,svc,secret 相关的资源变动会执行对应的处理函数。只看主要流程也就是ingress&svc,&ep资源本身的变动。暂时忽略cm跟secret的变动。
<a name="OWyP4"></a>
### Ingress
```go
//internal/ingress/controller/store/store.go

ingEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			ing, _ := toIngress(obj)
            // 记录事件
			recorder.Eventf(ing, corev1.EventTypeNormal, "Sync", "Scheduled for sync")
			// 将annoation的字段转换成一个 annotations.Ingress 对象，并且更新一下lister
			store.syncIngress(ing)
			// 创建一个事件放入 updateCh channel中，后面会被消费放入工作队列。这个消费的循环在上边提到过。
			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
    	// 这个函数比较大，后面写了分析。
		DeleteFunc: ingDeleteHandler,
		UpdateFunc: func(old, cur interface{}) {
			oldIng, _ := toIngress(old)
			curIng, _ := toIngress(cur)

			validOld := class.IsValid(oldIng)
			validCur := class.IsValid(curIng)
			if validOld && !validCur {
				klog.InfoS("removing ingress", "ingress", klog.KObj(curIng), "class", class.IngressKey)
				// 新的ingress不合法调用删除旧的直接返回
                ingDeleteHandler(old)
				return
			} else if validCur && !reflect.DeepEqual(old, cur) {
				// 新的合法并且与旧的不同，继续流程
				recorder.Eventf(curIng, corev1.EventTypeNormal, "Sync", "Scheduled for sync")
			} else {
                // 没变化就直接返回
				klog.V(3).InfoS("No changes on ingress. Skipping update", "ingress", klog.KObj(curIng))
				return
			}
			// 简单来说只新的ingress合法并且与旧的不同就把obj & event扔进channel
			store.syncIngress(curIng)
			updateCh.In() <- Event{
				Type: UpdateEvent,
				Obj:  cur,
			}
		},
	}
// 上面多次调用过这个函数
ingDeleteHandler := func(obj interface{}) {
		ing, ok := toIngress(obj)
		// 从lister中删除这个ingress
		store.listers.IngressWithAnnotation.Delete(ing)
		// 删除这个事件也入队
		updateCh.In() <- Event{
			Type: DeleteEvent,
			Obj:  obj,
		}
	}
```
<a name="roEF8"></a>
### ep
```go
	//internal/ingress/controller/store/store.go
	// 这个处理就相对简单很多直接入队就好了不用做任何复杂的转换
	epEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: CreateEvent,
				Obj:  obj,
			}
		},
		DeleteFunc: func(obj interface{}) {
			updateCh.In() <- Event{
				Type: DeleteEvent,
				Obj:  obj,
			}
		},
		UpdateFunc: func(old, cur interface{}) {
			oep := old.(*corev1.Endpoints)
			cep := cur.(*corev1.Endpoints)
			if !reflect.DeepEqual(cep.Subsets, oep.Subsets) {
				updateCh.In() <- Event{
					Type: UpdateEvent,
					Obj:  cur,
				}
			}
		},
	}
```
<a name="bnGsB"></a>
### service
```go
	//internal/ingress/controller/store/store.go
	//  这个处理更简单，只处理update的情况
	serviceHandler := cache.ResourceEventHandlerFuncs{
		UpdateFunc: func(old, cur interface{}) {
			oldSvc := old.(*corev1.Service)
			curSvc := cur.(*corev1.Service)

			if reflect.DeepEqual(oldSvc, curSvc) {
				return
			}

			updateCh.In() <- Event{
				Type: UpdateEvent,
				Obj:  cur,
			}
		},
	}
```
<a name="f9913"></a>
### workerQueue消费
infromer注册的handler会把变更事件丢到 updateCh 中，我们在start流程中，启动了一个for循环去获取这个updateCh中的事件，然后放入workerQueue中。同时我们也启动了一个gorountine 去消费workerQueue中的数据。我们接下来就去分析这个消费过程。
```go
//internal/ingress/controller/nginx.go
// 这个启动了消费的gorountine。
go n.syncQueue.Run(time.Second, n.stopCh)
//相应的消费函数是如下
// internal/task/queue.go
func (t *Queue) worker() {
	for {
        // 这个get能获取到队列中的一个数据，但是不会删除
		key, quit := t.queue.Get()
		ts := time.Now().UnixNano()
		item := key.(Element)
        // 如果上次的同步时间要比 event的时间要新的话跳过更新
		if t.lastSync > item.Timestamp {
			// 抛弃该元素，不再进行重试
            t.queue.Forget(key)
            // get 不会删除，这个done会真正的把元素删除
			t.queue.Done(key)
			continue
		}
        // 使用sync函数进行同步，这个我们下边分析，是整个流程的主体。 n.syncIngress()
		if err := t.sync(key); err != nil {
			klog.ErrorS(err, "requeuing", "key", item.Key)
			// 同步有问题，扔回到队列里。
            t.queue.AddRateLimited(Element{
				Key:       item.Key,
				Timestamp: time.Now().UnixNano(),
			})
		} else {
            // 同步成功，抛弃该元素
			t.queue.Forget(key)
            // 记录一下上次同步成功的时间
			t.lastSync = ts
		}
		// 把这个元素从队列中删除，不管成功与否都删除，失败的时候已经重新扔回队列了。
		t.queue.Done(key)
	}
}
```
​

syncIngress函数分析，我们先分析流程，然后层层深入具体的函数实现。
```
//internal/ingress/controller/controller.go
func (n *NGINXController) syncIngress(interface{}) error {
	// 获取token，获取不到时会阻塞
	n.syncRateLimiter.Accept()
	// 获取所有ingress资源
	ings := n.store.ListIngresses()
	hosts, servers, pcfg := n.getConfiguration(ings)
	// 配置没有变动直接返回
	if n.runningConfig.Equal(pcfg) {
		klog.V(3).Infof("No configuration change detected, skipping backend reload")
		return nil
	}
	// 判断配是否需要nginx reload
	if !n.IsDynamicConfigurationEnough(pcfg) {
		klog.InfoS("Configuration changes detected, backend reload required")

		hash, _ := hashstructure.Hash(pcfg, &hashstructure.HashOptions{
			TagName: "json",
		})

		pcfg.ConfigurationChecksum = fmt.Sprintf("%v", hash)
		// 有变动之后使用新的配置，渲染配置文件然后nginx -s reload
		err := n.OnUpdate(*pcfg)
		}

	isFirstSync := n.runningConfig.Equal(&ingress.Configuration{})
  // 第一次更新先sleep一会等nginx启动
	if isFirstSync {
		// For the initial sync it always takes some time for NGINX to start listening
		// For large configurations it might take a while so we loop and back off
		klog.InfoS("Initial sync, sleeping for 1 second")
		time.Sleep(1 * time.Second)
	}
	// 等待成功
	err := wait.ExponentialBackoff(retry, func() (bool, error) {
  	// 动态配置更新,主要是将数据信息注册到nginx-lua中供负载均衡使用。
		err := n.configureDynamically(pcfg)
		if err == nil {
			klog.V(2).Infof("Dynamic reconfiguration succeeded.")
			return true, nil
		}
		return false, err
	})

	n.runningConfig = pcfg

	return nil
}
```
configureDynamically 这个函数主要是配置nginx-lua模块需要的一些信息，供负载均衡使用。
```
// internal/ingress/controller/nginx.go
func (n *NGINXController) configureDynamically(pcfg *ingress.Configuration) error {
	//	判断是否发生变化
	backendsChanged := !reflect.DeepEqual(n.runningConfig.Backends, pcfg.Backends)
	if backendsChanged {
  	// 变化就配置一下backend的信息
		err := configureBackends(pcfg.Backends)
		if err != nil {
			return err
		}
	}
	return nil
}


func configureBackends(rawBackends []*ingress.Backend) error {
	backends := make([]*ingress.Backend, len(rawBackends))
	// 省略处理格式的函数
  // 这里调用了nginx-lua提供的api把信息发送到nginx-lua,然后供负载均衡使用
	statusCode, _, err := nginx.NewPostStatusRequest("/configuration/backends", "application/json", backends)
	if err != nil {
		return err
	}

	return nil
}
```
​<br />
<a name="tfluE"></a>
## lua部分
<a name="oyzUi"></a>
### 测试用例
[https://github.com/phantooom/k8s-valid-demo/tree/main/nginx-ingress-canary](https://github.com/phantooom/k8s-valid-demo/tree/main/nginx-ingress-canary)
```
apiVersion: v1
kind: Service
metadata:
  name: service-v1
spec:
  selector:
    app: v1
  ports:
    - protocol: TCP
      port: 8080
      targetPort: 8080
--
apiVersion: v1
kind: Service
metadata:
  name: service-v2
spec:
  selector:
    app: v2
  ports:
    - protocol: TCP
      port: 8080
      targetPort: 8080
--
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  name: ingress-v1
spec:
  rules:
  - host: canary.test.com
    http:
      paths:
      - backend:
          serviceName: service-v1
          servicePort: 8080
        path: /
--
apiVersion: extensions/v1beta1
kind: Ingress
metadata:
  annotations:
    nginx.ingress.kubernetes.io/canary: "true"
    nginx.ingress.kubernetes.io/canary-weight: "50"
  name: ingress-v2
spec:
  rules:
  - host: canary.test.com
    http:
      paths:
      - backend:
          serviceName: service-v2
          servicePort: 8080
        path: /
```


<a name="Uc6RN"></a>
### /configuration/backends 接口
这个接口是直接go部分会直接调用的。我们看下实现
```
// rootfs/etc/nginx/lua/configuration.lua
function _M.call()
	// 这里是处理 /configuration/backends的部分，转到了 handle_backends 方法处理
  if ngx.var.request_uri == "/configuration/backends" then
    handle_backends()
    return
  end
end


local function handle_backends()
	// 取得请求的body
  local backends = fetch_request_body()
  // 放到 configuration_data 这个里面
  local success, err = configuration_data:set("backends", backends)
  
  ngx.update_time()
  local raw_backends_last_synced_at = ngx.time()
  // 记录更新时间
  success, err = configuration_data:set("raw_backends_last_synced_at", raw_backends_last_synced_at)
  ngx.status = ngx.HTTP_CREATED
end

```
```
// 这是存储的数据结构
[{
	"name": "default-service-v1-8080",
	"service": {
		"metadata": {
			"creationTimestamp": null
		},
		"spec": {
			"ports": [{
				"protocol": "TCP",
				"port": 8080,
				"targetPort": 8080
			}],
			"selector": {
				"app": "v1"
			},
			"clusterIP": "192.168.7.57",
			"type": "ClusterIP",
			"sessionAffinity": "None"
		},
		"status": {
			"loadBalancer": {}
		}
	},
	"port": 8080,
	"sslPassthrough": false,
	"endpoints": [{
		"address": "10.128.32.53",
		"port": "8080"
	}],
	"sessionAffinityConfig": {
		"name": "",
		"mode": "",
		"cookieSessionAffinity": {
			"name": ""
		}
	},
	"upstreamHashByConfig": {
		"upstream-hash-by-subset-size": 3
	},
	"noServer": false,
	"trafficShapingPolicy": {
		"weight": 0,
		"header": "",
		"headerValue": "",
		"headerPattern": "",
		"cookie": ""
	},
	"alternativeBackends": ["default-service-v2-8080"]
}, {
	"name": "default-service-v2-8080",
	"service": {
		"metadata": {
			"creationTimestamp": null
		},
		"spec": {
			"ports": [{
				"protocol": "TCP",
				"port": 8080,
				"targetPort": 8080
			}],
			"selector": {
				"app": "v2"
			},
			"clusterIP": "192.168.1.120",
			"type": "ClusterIP",
			"sessionAffinity": "None"
		},
		"status": {
			"loadBalancer": {}
		}
	},
	"port": 8080,
	"sslPassthrough": false,
	"endpoints": [{
		"address": "10.128.32.173",
		"port": "8080"
	}],
	"sessionAffinityConfig": {
		"name": "",
		"mode": "",
		"cookieSessionAffinity": {
			"name": ""
		}
	},
	"upstreamHashByConfig": {
		"upstream-hash-by-subset-size": 3
	},
	"noServer": true,
	"trafficShapingPolicy": {
		"weight": 50,
		"header": "",
		"headerValue": "",
		"headerPattern": "",
		"cookie": ""
	}
}, {
	"name": "upstream-default-backend",
	"port": 0,
	"sslPassthrough": false,
	"endpoints": [{
		"address": "127.0.0.1",
		"port": "8181"
	}],
	"sessionAffinityConfig": {
		"name": "",
		"mode": "",
		"cookieSessionAffinity": {
			"name": ""
		}
	},
	"upstreamHashByConfig": {},
	"noServer": false,
	"trafficShapingPolicy": {
		"weight": 0,
		"header": "",
		"headerValue": "",
		"headerPattern": "",
		"cookie": ""
	}
}]
```
<a name="mV8pt"></a>
### sync_backends 任务
```
// rootfs/etc/nginx/lua/balancer.lua
// 初始化的时候我们可以看到注册了这个sync_backends函数定期执行
function _M.init_worker()
  sync_backends()
  ok, err = ngx.timer.every(BACKENDS_SYNC_INTERVAL, sync_backends)
end


local function sync_backends()
	// 没获取到backend的数据那么 balancers 改成空的dict
  local backends_data = configuration.get_backends_data()
  if not backends_data then
    balancers = {}
    return
  end
	// 解析 backends_data 为json
  local new_backends, err = cjson.decode(backends_data)
  if not new_backends then
    ngx.log(ngx.ERR, "could not parse backends data: ", err)
    return
  end

  local balancers_to_keep = {}
  // 将新的backend循环放入 balancers_to_keep dict中
  for _, new_backend in ipairs(new_backends) do
  	// 这里主要是判断是否是带有externalname的服务 https://kubernetes.io/docs/concepts/services-networking/service/#externalname
    if is_backend_with_external_name(new_backend) then
      local backend_with_external_name = util.deepcopy(new_backend)
      // 放入 backends_with_external_name 这个dict中
      backends_with_external_name[backend_with_external_name.name] = backend_with_external_name
    else
    	// 将backend放到balancers中，下边会详细分析比较重要。
      sync_backend(new_backend)
    end
    balancers_to_keep[new_backend.name] = true
  end
	// 这个是循环所有的balancers 然后找出过期的内容进行删除
  for backend_name, _ in pairs(balancers) do
    if not balancers_to_keep[backend_name] then
      balancers[backend_name] = nil
      backends_with_external_name[backend_name] = nil
    end
  end
  backends_last_synced_at = raw_backends_last_synced_at
end


// 这个方法实际上就是会做的就是将backend 初始化放入 balancers dict中
// 中间会负责把externalname 解析到ip，会配置指定的负载均衡算法
local function sync_backend(backend)
	// 如果是externalname 的形式需要解析成ip
  if is_backend_with_external_name(backend) then
    backend = resolve_external_names(backend)
  end
	// 获取负载均衡的算法，并且初始化相关的实现
  local implementation = get_implementation(backend)
  // 获取老的balancer对象
  local balancer = balancers[backend.name]
	// 不存在就新建
  if not balancer then
    balancers[backend.name] = implementation:new(backend)
    return
  end
	// 不相等的话就证明负载均衡算发变了就重新初始化一下。
  if getmetatable(balancer) ~= implementation then
    ngx.log(ngx.INFO,
        string.format("LB algorithm changed from %s to %s, resetting the instance",
                      balancer.name, implementation.name))
    balancers[backend.name] = implementation:new(backend)
    return
  end
	// 调用sync的方法,各个负载均衡可以自己实现也可以用balancer.resty的实现
  // 主要判断endpont是否发生变化，如果发生变化要使用最新的初始化一下。
  balancer:sync(backend)
end

```


<a name="milzV"></a>
### 用户访问


```
    // rootfs/etc/nginx/template/nginx.tmpl
    // 用户的流量到达nginx匹配到对应的location之后proxy_pass 到 upstream_balancer
    // 使用balancer.balance()这个方法选择对应的backend
    upstream upstream_balancer {
        ### Attention!!!
        #
        # We no longer create "upstream" section for every backend.
        # Backends are handled dynamically using Lua. If you would like to debug
        # and see what backends ingress-nginx has in its memory you can
        # install our kubectl plugin https://kubernetes.github.io/ingress-nginx/kubectl-plugin.
        # Once you have the plugin you can use "kubectl ingress-nginx backends" command to
        # inspect current backends.
        #
        ###

        server 0.0.0.1; # placeholder

        balancer_by_lua_block {
          balancer.balance()
        }

        {{ if (gt $cfg.UpstreamKeepaliveConnections 0) }}
        keepalive {{ $cfg.UpstreamKeepaliveConnections }};

        keepalive_timeout  {{ $cfg.UpstreamKeepaliveTimeout }}s;
        keepalive_requests {{ $cfg.UpstreamKeepaliveRequests }};
        {{ end }}
    }
```
```

// rootfs/etc/nginx/lua/balancer.lua
function _M.balance()
	// 这个会去获取 balancer 对象。
  local balancer = get_balancer()
  if not balancer then
    return
  end
	// balancer 的balance方法会在之前初始化的负载均衡算法中实现，具体就是选择一个ip:port
  local peer = balancer:balance()
  if not peer then
    ngx.log(ngx.WARN, "no peer was returned, balancer: " .. balancer.name)
    return
  end
	// 设置重试次数
  ngx_balancer.set_more_tries(1)
	// 这个就是把上文选择到的节点设置成当前的upstream
  local ok, err = ngx_balancer.set_current_peer(peer)
  if not ok then
    ngx.log(ngx.ERR, "error while setting current upstream peer ", peer,
            ": ", err)
  end
end

// 获取balancer对象的流程
local function get_balancer()
  local backend_name = ngx.var.proxy_upstream_name // default-service-v1-8080
	// 没获取到直接返回
  local balancer = balancers[backend_name] // default-service-v1-8080
  if not balancer then
    return nil
  end
	// 获取到了这边还要在判断一次是否要给返回一个金丝雀的balancer。所以金丝雀是在这边实现的
  // 如果需要路由到金丝雀则把backend替换成金丝雀的balancer
  // 这个其实就是从 alternativeBackends中拿第一个name然后从dict中再取一次金丝雀的backend
  if route_to_alternative_balancer(balancer) then
    local alternative_backend_name = balancer.alternative_backends[1]
    ngx.var.proxy_alternative_upstream_name = alternative_backend_name // default-service-v2-8080
    balancer = balancers[alternative_backend_name]
  end

  return balancer
end
```

# 总结

整体来说大体的逻辑还是标准的controller编写流程，但是具体到写的感觉写的并不是很好，不少没有用到的老代码，逻辑层级过多。有问题不太好排查。只能记住关键的流程点位，有问题结合具体版本进行分析把。
