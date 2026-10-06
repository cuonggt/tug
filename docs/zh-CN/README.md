<!-- translated from docs/README.md at ba93a85f29e8 -->

# tug 指南

tug 是一个 Web 框架，面向前端使用 [Inertia.js](https://inertiajs.com) 的 Go 应用：Go 处理函数用 props 渲染 React、Vue 或 Svelte 页面，中间无需 API，整个应用以单个二进制文件发布。本指南从新建应用一直讲到部署上线。其余细节请参阅 [pkg.go.dev](https://pkg.go.dev/github.com/cuonggt/tug) 上各个包自己的文档。

1. [快速上手](getting-started.md)：安装 tug，创建并运行应用，再添加一个页面和一个表单。
2. [路由与处理函数](../routing.md)：App、路由、绝对链接与签名链接、路由组、中间件（其中包括安全响应头）、响应了请求的路由、代理之后的客户端地址、`Ctx`、文件、下载、流与事件、请求绑定，以及错误。
3. [页面](../pages.md)：Inertia 页面及其 props、根模板及其 nonce、延后计算的 props、共享 props、重定向、页面上的下载与事件、错误页面、Vite，以及 Inertia 的 DevTools。
4. [服务端渲染](../ssr.md)：首次访问时，由应用旁边的 Node 在服务端渲染页面，以及 `ssr` 包。
5. [表单与会话](../forms.md)：验证、应用自己的规则、离开字段时就检查该字段的表单、flash 消息、会话，以及 CSRF。
6. [多语言](../languages.md)：用请求的语言显示 tug 和应用的文字，每种语言一个文件；`lang` 包，以及 `tug lang`。
7. [文件](../files.md)：上传的文件，按大小和实际类型检查，用 `storage` 包保存在应用的磁盘或 S3 上，以及指向它们的公开链接或签名链接。
8. [账户](../auth.md)：认证脚手架，可用 SQLite、Postgres 或 MySQL，及其 API 令牌、管理员和通知；`auth` 和 `mail` 包，后者支持抄送、附件和一键退订链接。
9. [数据库迁移](../migrations.md)：数据库的表由 SQL 文件创建和修改，每个文件只运行一次，在应用启动时按顺序运行，也可以通过应用的 `migrate` 命令运行；用 `tug migrate new` 创建下一个迁移；以及 `migrate` 包。
10. [授权](../authorization.md)：用户可以对某个对象做什么，`auth` 包的权限（ability），拒绝时返回说明原因的 403，以及在页面的 props 中给出当前用户可以做什么。
11. [加密](../encryption.md)：应用的密钥，tug 用它加密和签名的内容，用来加密应用自身数据的 `crypt` 包，以及密钥轮换。
12. [后台任务](../jobs.md)：`queue` 包，用于比请求持续更久的工作：失败时重新运行；按计划运行，也可以按指定的时区；无论被请求多少次，都只排队一次；或者在所有实例上合计，限制同时运行多少个、每秒运行多少个；并告知任务何时彻底失败，以及每次运行的情况。
13. [缓存](../cache.md)：`cache` 包，用于计算起来很慢的结果，存放在应用的每个实例都能找到的地方；以及锁，用于不能同时运行两次的操作。
14. [广播](../broadcasting.md)：`broadcast` 包，频道上的事件，由应用的数据库送到每个实例上打开的页面，页面随即重新加载变化的部分。
15. [TypeScript](../typescript.md)：`tug gen` 根据 Go 代码写出的类型。
16. [测试](../testing.md)：`tugtest` 包，即供 Go 测试使用的 Inertia 客户端，用来测试应用的页面、表单、上传和登录；用来测试邮件的 `mailtest`；以及应用的事件。
17. [部署](../deployment.md)：单个二进制文件、Dockerfile、环境变量，规定浏览器可以如何对待应用页面的响应头（其中包括 Content-Security-Policy），日志，以及指标。
18. [命令行工具](../cli.md)：`tug new`、`tug dev`、`tug gen`、`tug lang`、`tug migrate`、`tug build` 和 `tug key` 的完整说明。

[路线图](../roadmap.md)讲述了 tug 是如何构建的、背后的决策，以及接下来的计划；[基准测试](../benchmarks.md)则给出 tug 在每个请求上的开销、用它构建的应用运行起来需要多少资源，以及这些数据是如何测得的。
