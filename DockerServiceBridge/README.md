# Docker 服务订阅桥

这个小服务把本机 Docker 容器里的 `homepage.*` labels 转成个人工具台可以订阅的服务入口列表。

默认只监听本机 loopback：

```bash
go run . -addr 127.0.0.1:17321
```

然后在个人工具台的“设置 -> 服务订阅”里添加：

```text
http://127.0.0.1:17321/v1/services
```

如果从线上页面读取这个地址，浏览器可能会提示“允许访问本机网络”或类似授权；选择允许后，服务页才能读取本机 Docker 入口。

## 用 Docker 镜像运行

如果不想在本机安装 Go，可以使用已经发布的镜像。宿主机端口只发布到 `127.0.0.1`，避免被局域网其它设备访问：

```bash
docker pull etng/tk-docker-service-bridge:latest
docker rm -f tooldkit-docker-service-bridge 2>/dev/null || true
docker run -d \
  --name tooldkit-docker-service-bridge \
  -p 127.0.0.1:17321:17321 \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  etng/tk-docker-service-bridge:latest
```

然后在个人工具台的“设置 -> 服务订阅”里添加：

```text
http://127.0.0.1:17321/v1/services
```

如果浏览器提示是否允许访问本机网络，请选择允许。

也可以在仓库根目录通过 compose 同时启动静态站和桥服务：

```bash
docker compose up -d
```

默认静态站地址是：

```text
http://127.0.0.1:8088/
```

如果镜像发布在自己的 Docker Hub 命名空间，可以设置：

```bash
DOCKERHUB_NAMESPACE=your-dockerhub-name docker compose up -d
```

查看运行状态：

```bash
curl http://127.0.0.1:17321/healthz
docker logs tooldkit-docker-service-bridge
```

停止服务：

```bash
docker rm -f tooldkit-docker-service-bridge
```

## Docker labels

容器需要显式声明这些 labels 才会被展示：

```yaml
labels:
  - homepage.group=SelfHost
  - homepage.name=example-service
  - homepage.href=http://localhost:58080
  - homepage.description=A local service entry.
  - homepage.icon=server
```

可选字段：

- `homepage.url`：可替代 `homepage.href`
- `homepage.title`：可替代 `homepage.name`
- `homepage.enabled=false`：忽略这个容器

## 安全设置

服务默认只允许这些页面跨域读取：

```text
https://etng.github.io
http://127.0.0.1:5173
http://localhost:5173
```

可以用 `-origins` 覆盖：

```bash
go run . -origins "https://etng.github.io,http://127.0.0.1:5173"
```

如果想再加一层本机 token：

```bash
go run . -token "your-random-token"
```

订阅地址填写：

```text
http://127.0.0.1:17321/v1/services?token=your-random-token
```

默认不启用自签名 HTTPS。现代浏览器可以在用户授权后让 HTTPS 页面访问 `http://127.0.0.1` 这类 loopback 地址；自签名证书反而需要用户手动信任证书，试用成本更高。
