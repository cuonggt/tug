<!-- translated from docs/getting-started.md at 15ea1f8eac10 -->

# 快速上手

本页会新建一个应用并运行它，为它添加一个页面和一个表单并测试它们，最后把应用构建成单个二进制文件。

## 环境要求

- Go 1.26 或更高版本。
- Node.js 22.12 或更高版本，或者 Node 20 中的 20.19 或更高版本：构建前端的 Vite 8 需要其中之一。应用的 Dockerfile 用 Node 24 构建。
- npm，随 Node 一起安装。`tug new` 会用它安装应用的依赖包。

## 安装 tug

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug version
```

`go install` 会把 `tug` 放到 `$(go env GOPATH)/bin`，设置了 `$GOBIN` 时则放到 `$GOBIN`，这个目录需要在你的 `PATH` 中。`tug version` 会打印已安装的版本，例如 `tug v0.39.1`。`tug new` 创建的应用会依赖这个版本。

## 创建应用

```sh
tug new blog
cd blog
```

`tug new` 会创建目录 `blog`（它必须尚不存在，或者是空的），并在其中放入一个可以直接运行的应用：

- 脚手架的文件，以及一个以目录命名的 Go 模块，可以用 `-module github.com/you/blog` 改名；
- 一个 `.env` 文件，其中有新生成的 `APP_KEY`，即用来加密会话 cookie 的 32 个随机字节，还有 `APP_DEBUG=true`，它会在页面上显示服务器错误的详细信息：错误本身、它从哪里来，以及请求；
- Go 模块（通过 `go mod tidy`）和前端的依赖包（通过 `npm install`）；
- 应用的页面和路由的 TypeScript，位于 `resources/js/tug`，为了写出它们，`tug new` 会先构建一次应用。

要创建带账户的应用，加上 `-auth`：

```sh
tug new -auth blog
```

这会在第一套脚手架之上再铺一套。用户可以注册并验证邮箱；可以登录，开启双因素验证后，还要输入身份验证器应用中的验证码；可以通过邮件发来的链接重置忘记的密码；还可以在设置中修改个人资料和头像、密码以及外观。用户数据存储在 SQLite 的 `app.db` 中，头像存放在 `files/`，前端使用 Tailwind 和 shadcn/ui。加上 `-postgres` 或 `-mysql` 时，用户数据改为存储在 Postgres 或 MySQL 中，数据库由应用的 `compose.yaml` 运行：在 `tug dev` 之前先执行 `docker compose up -d`。这些代码都归应用自己所有，可以按应用的需要修改：处理函数在 `auth.go` 及其旁边的文件中，数据库的 SQL 在 `migrations/`（用来创建数据表）和各个 `_db.go` 文件中（见[数据库迁移](../migrations.md)）。在设置 `MAIL_HOST` 之前，邮件不会发出：在 `tug dev` 下，邮件保存在应用自带的邮箱中，位于 `/_tug/mail`，它会像邮件客户端那样显示每封邮件；应用的输出中也会为每封邮件打印一行，附上它的链接。详见[账户](../auth.md)。

如果需要在服务端渲染页面，让搜索引擎能够读取，并让页面在脚本运行之前就能显示，可以在上面任一命令中加上 `-ssr`：页面由应用旁边的 Node 渲染。详见[服务端渲染](../ssr.md)。

前端默认是 React，加上 `-vue` 或 `-svelte` 则是 Vue 或 Svelte：应用相同，Go 代码也相同，只是页面写成 `.vue` 或 `.svelte` 组件，而 React 的页面是 `.tsx`；`-auth` 和 `-ssr` 同样可以与它们一起使用。本页接下来以基础脚手架为例，前端为 React。

## 运行应用

```sh
tug dev
```

```
tug  │ serving http://localhost:8080, built in 268ms
```

打开 http://localhost:8080。`tug dev` 会运行两个服务器，并把它们的输出和自己的输出一起显示，每一行都标有 `tug`、`app` 或 `vite`：

- **Vite 的开发服务器**，即 `npm run dev`，运行在 `localhost:5173`，负责向浏览器提供前端的模块。
- **应用**，即浏览器访问的 Go 服务器，构建到 `.tug/app`。8080 端口被占用时，`tug dev` 会改用下一个空闲端口，并给出提示。也可以在 `.env` 或环境变量中用 `ADDR` 或 `PORT` 指定端口。

页面来自 Go 服务器，页面的脚本则来自 Vite：Vite 运行期间，`vite.config.ts` 中的插件会把 Vite 的地址写入 `public/hot`，Go 服务器每次渲染页面时都会读取它。

开发过程中：

- **Go 文件、`go.mod` 和 `go.sum`，以及模板**（`.html`、`.tmpl`、`.gohtml`）：tug 会重新构建应用，在 TypeScript 类型有变化时重新写入，然后重启应用并刷新浏览器。
- **构建失败时**：错误显示在 `app` 下，上一次构建成功的应用会继续运行，直到某次保存修复了错误。
- **前端**，即 `resources/` 下的文件：Vite 会把改动发送给浏览器，React 组件会就地更新，并保留其状态。
- **`.env`**：只在 `tug dev` 启动时读取。用 Ctrl-C 停止它，然后重新启动。

详见[命令行工具](../cli.md#tug-dev)。

## 应用里有什么

```
blog/
├── main.go              服务器：它的页面、路由和处理函数
├── flash.go             flash 数据的键，tug gen 会为它们生成类型
├── main_test.go         它的测试
├── app.html             每个页面都渲染在这个 HTML 中
├── resources/
│   ├── css/app.css
│   └── js/
│       ├── app.tsx      前端的入口：为每个页面找到它的组件
│       ├── Layout.tsx   每个页面外围的内容，flash 消息也在其中
│       ├── pages/       每个页面一个组件：Home.tsx 和 Error.tsx
│       └── tug/         由 tug gen 写出，并非手写：pages.ts、routes.ts
├── public/              会放进二进制文件；Vite 构建到 public/build
├── vite.config.ts       Vite 的配置，以及与 tug dev 配合的插件
├── go.mod, package.json, tsconfig.json
├── .env                 供 tug 命令读取，应用本身不读；不提交
├── .env.example         与 .env 相同但不含密钥，用于提交
├── Dockerfile           把应用作为单个二进制文件放进 distroless 镜像
└── README.md
```

`main.go` 分为两部分。`main` 从环境变量中读取应用的配置（用 `session.KeysFromEnv` 读取 `APP_KEY`，用 `tug.ConfigFromEnv` 读取 `ADDR`、`PORT` 和 `APP_DEBUG`），然后运行应用。`newApp` 用这些配置把应用组装起来：Vite 的标签、外面包着 `app.html` 的 Inertia 页面、会话、错误页面、中间件、在 `/build/` 下提供前端构建产物的路由，以及应用自己的路由。测试会用它们自己的配置调用 `newApp`。

## 添加页面

页面是 `resources/js/pages` 中的一个 React 组件，加上渲染它的 Go 代码：一个由其 props 组成的结构体、一个把它们与组件关联起来的 `tug.Page`、一个处理函数，以及一条路由。下面这个页面列出博客的文章。把以下代码加到 `main.go` 中，并在它的 import 中加上 `slices` 和 `sync`：

```go
// Post 是博客上的一篇文章。
type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// posts 把文章保存在内存中，所以应用停止后它们就没了。
// 请求是并发处理的，所以列表带有一把锁。
type posts struct {
	mu   sync.Mutex
	list []Post
}

// PostsIndexProps 是 resources/js/pages/Posts/Index.tsx 的 props。
type PostsIndexProps struct {
	Posts []Post `json:"posts"`
}

var PostsIndex = tug.Page[PostsIndexProps]("Posts/Index")

func (p *posts) index(c *tug.Ctx) error {
	p.mu.Lock()
	list := slices.Clone(p.list)
	p.mu.Unlock()
	return PostsIndex.Render(c, PostsIndexProps{Posts: list})
}
```

然后在 `newApp` 中，在其他路由下面加上：

```go
	p := &posts{}
	app.Get("/posts", p.index).Name("posts.index")
```

`tug.Page[PostsIndexProps]("Posts/Index")` 声明了组件 `resources/js/pages/Posts/Index.tsx` 及其接收的 props：`Render` 只接受 `PostsIndexProps`，别的都不接受。json 标签决定了组件拿到的 props 叫什么名字；空列表会以 `[]` 发出，绝不会是 `null`。路由的名字 `posts.index` 让 Go 和前端都能引用这条路由，而不必写出它的路径。文章列表是在 `newApp` 中创建的，所以每个测试中的应用一开始都没有文章。

保存后，`tug dev` 会构建应用并重新写入类型：

```
tug  │ main.go changed
tug  │ wrote resources/js/tug/pages.ts, resources/js/tug/routes.ts
tug  │ serving http://localhost:8080, built in 844ms
```

现在 `pages.ts` 中有了这个页面的 props，用 TypeScript 写成：

```ts
export interface Post {
  id: number
  title: string
}

export interface PostsIndexProps {
  posts: Post[]
}
```

组件放在 `resources/js/pages/Posts/Index.tsx` 中：

```tsx
import { Head } from '@inertiajs/react'
import Layout from '../../Layout'
import type { PageProps } from '../../tug/pages'

export default function Index({ posts }: PageProps<'Posts/Index'>) {
  return (
    <Layout>
      <Head title="Posts" />
      <h1>Posts</h1>
      <ul>
        {posts.map((post) => (
          <li key={post.id}>{post.title}</li>
        ))}
      </ul>
    </Layout>
  )
}
```

`PageProps<'Posts/Index'>` 包括页面自己的 props，以及所有页面共享的 props，例如 `appName`，它由 `newApp` 通过 `pages.Share` 共享。要从首页链接到这个页面，在 `Home.tsx` 中把 `Link` 加到从 `@inertiajs/react` 导入的内容里，并在问候语下面加上：

```tsx
<Link href={route('posts.index')}>Posts</Link>
```

`route()` 知道应用的所有命名路由。传入不存在的名字（例如 `route('post.index')`）无法通过类型检查；调用带通配符的路由却不传入它的值，同样无法通过。值按名字传入：对于 `/posts/{id}` 路由，`route('posts.show', { id: 42 })` 就是 `/posts/42`。Vite 在提供文件时不检查类型；检查类型的是你的编辑器，以及 `npm run typecheck` 和 `tug build`。

## 添加表单

这个表单用来添加文章。在 Go 中，它是一个描述表单所发送内容的结构体，每个字段的规则写在它的 `validate` 标签中，再加上一个处理函数：

```go
// PostInput 是新建文章的表单发送的内容。
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
}

func (p *posts) store(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // 带着错误回到表单
	}
	p.mu.Lock()
	p.list = append(p.list, Post{ID: len(p.list) + 1, Title: in.Title})
	p.mu.Unlock()
	c.Flash("success", "Post created")
	return c.RedirectRoute("posts.index")
}
```

以及它的路由，加在 `newApp` 中：

```go
	app.Post("/posts", p.store).Name("posts.store")
```

`c.BindValid` 用请求填充 `in`，并按 `validate` 标签检查它，这些标签使用 go-playground/validator 的规则。如果有问题，它会返回错误，处理函数再把它们返回：浏览器回到表单，页面在它的 `errors` prop 中拿到这些错误，例如“title is required”或“title must be at most 80 characters”。`c.Flash` 为下一个显示的页面留下一条消息，脚手架的 `Layout` 会显示它。`c.RedirectRoute` 把浏览器重定向到一个命名路由，在 POST 之后使用 303，这样浏览器会接着用 GET 访问。

在 `Index.tsx` 中，在 `Head` 旁边导入 `Form`，并导入 `route`：

```tsx
import { Form, Head } from '@inertiajs/react'
import { route } from '../../tug/routes'
```

然后在列表下面加上表单：

```tsx
<Form action={route('posts.store')} method="post" resetOnSuccess className="form">
  {({ errors, processing }) => (
    <>
      <label>
        Title
        <input name="title" />
      </label>
      {errors.title && <p className="error">{errors.title}</p>}
      <button type="submit" disabled={processing}>
        Add post
      </button>
    </>
  )}
</Form>
```

Inertia 的 `<Form>` 按输入框的 `name` 发送它们，这些 name 就是 Go 结构体字段的 json 名；它还会以同样的名字把 `errors` 交给它的子元素。文章添加成功后，`resetOnSuccess` 会清空输入框。`form` 和 `error` 这两个类来自脚手架的 CSS。

脚手架的首页还会在表单发送之前，在用户离开字段时就检查该字段：它的输入框失去焦点时会调用 `validate('name')`，`BindValid` 会用该字段的错误回应这个请求，即一个 Precognition 请求，处理函数到此为止。这里同样可以做到：使用表单的子元素函数提供的 `validate`，并在输入框上加上 `onBlur={() => validate('title')}`。其余内容见[表单与会话](../forms.md)：处理函数自己的检查、错误包、flash 数据，以及会话。

## 测试应用

```sh
go test ./...
```

脚手架中的 `main_test.go` 用 `tugtest` 包，按 Inertia 客户端使用应用的方式测试应用，无需浏览器，也无需构建前端。`newClient` 用 `newApp`、一个空的构建和它自己的密钥创建应用，并返回一个 `tugtest.Client`，相当于一个打开了这个应用的浏览器。它的每次访问都以 JSON 形式拿到页面，即页面的组件和 props；它还会在一次次访问之间保留会话 cookie，表单的错误和 flash 消息正是靠这个 cookie 带到下一个页面的。下面是文章的两个测试，写在 `main_test.go` 中：

```go
func TestANewPostIsInTheList(t *testing.T) {
	c := newClient(t)
	c.Get("/posts")
	r := c.Post("/posts", map[string]any{"title": "Hello, tug"})
	if r.Location() != "/posts" {
		t.Fatalf("got %v, want back to the list", r)
	}
	r = r.Follow()
	if posts := tugtest.Props(r, PostsIndex).Posts; len(posts) != 1 || r.Page.Flash["success"] != "Post created" {
		t.Fatalf("posts %v, flash %v", posts, r.Page.Flash)
	}
}

func TestAPostWithoutATitleComesBackWithWhatToFix(t *testing.T) {
	c := newClient(t)
	c.Get("/posts")
	if errs := c.Post("/posts", map[string]any{"title": ""}).Follow().Errors(); errs["title"] != "title is required" {
		t.Fatalf("errors %v", errs)
	}
}
```

`c.Post` 返回应用的响应，在这里是一个重定向；`Follow` 会像浏览器那样跟随它到下一个页面：文章列表，或者对于没有标题的文章，回到发送表单的那个页面，并带上错误。`tugtest.Props` 把页面的 props 读入其 `tug.Page` 所声明的 `PostsIndexProps`，遇到其他页面则让测试失败。详见[测试](../testing.md)。

`npm run typecheck` 会对照 `tug gen` 写出的类型检查前端。

## 构建应用

```sh
tug build
```

`tug build` 会写入类型，对前端进行类型检查，用 Vite 把前端构建到 `public/build`，再把应用构建成单个二进制文件 `./blog`，前端就在其中：`main.go` 嵌入了 `public/`。脚手架应用的二进制文件约 10 MB，并且是静态的，因此不需要安装任何其他东西就能运行。

这个二进制文件从环境变量读取配置，而不是从 `.env` 读取：

```sh
APP_KEY=base64:... ADDR=127.0.0.1:8080 ./blog
```

没有 `APP_KEY` 时，它会停止运行并说明原因。部署的应用需要自己的密钥：`tug key` 会打印一个，`APP_KEY` 可以原样使用它。应用监听 `ADDR`；没有设置时监听 `PORT`，Cloud Run 和 Fly.io 等平台会设置它；两者都没有时监听 `:8080`。不要设置 `APP_DEBUG`，这样 500 错误的详细信息只会留在日志中。

应用的 `Dockerfile` 用 Node 构建前端，用 Go 构建二进制文件，然后把这个二进制文件单独放到一个 distroless 镜像上：

```sh
docker build -t blog .
docker run -p 8080:8080 -e APP_KEY=base64:... blog
```

构建镜像时不会运行 `tug`：其中的 Vite 构建使用目录中已有的 `resources/js/tug`，脚手架的 `.gitignore` 没有忽略它，它应该和生成它的 Go 代码一起提交。其余内容见[部署](../deployment.md)：镜像、环境变量，以及代理。

## 下一步

- [路由与处理函数](../routing.md)：路由、路由组、中间件、`Ctx`、绑定，以及错误。
- [页面](../pages.md)：props 与 prop 类型、共享 props、重定向、错误页面，以及 Vite。
- [表单与会话](../forms.md)：验证、Precognition、flash、会话和 CSRF。
- [账户](../auth.md)：认证脚手架，以及 `auth` 和 `mail` 包。
- [TypeScript](../typescript.md)：`tug gen` 根据 Go 类型写出的内容。
- [部署](../deployment.md)：`tug build`、Dockerfile，以及环境变量。
- [命令行工具](../cli.md)：每条命令的完整说明。

[`examples/inertia`](../../examples/inertia/main.go) 是一个更大的应用：可以创建、编辑和删除文章，有一个延迟 prop，还有一个滚动时加载更多内容的列表。
