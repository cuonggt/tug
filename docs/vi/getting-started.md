<!-- translated from docs/getting-started.md at 9c124cf969af -->

# Bắt đầu

Trên trang này, bạn sẽ tạo một ứng dụng mới và chạy nó, thêm vào đó một
trang và một form, kiểm thử chúng, rồi build ứng dụng thành một binary duy
nhất.

## Bạn cần gì

- Go 1.26 trở lên.
- Node.js 22.12 trở lên, hoặc 20.19 trở lên nếu dùng Node 20: Vite 8, công
  cụ build frontend, cần một trong hai phiên bản đó. Dockerfile của ứng
  dụng build bằng Node 24.
- npm, đi kèm với Node. `tug new` dùng nó để cài các gói của ứng dụng.

## Cài đặt tug

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug version
```

`go install` đặt `tug` vào `$(go env GOPATH)/bin`, hoặc vào `$GOBIN` nếu
biến này có giá trị, và thư mục đó cần nằm trong `PATH` của bạn.
`tug version` in ra phiên bản đã cài, chẳng hạn `tug v0.39.0`. Các ứng
dụng do `tug new` tạo ra đều yêu cầu đúng phiên bản đó.

## Tạo một ứng dụng

```sh
tug new blog
cd blog
```

`tug new` tạo thư mục `blog`, thư mục này phải chưa tồn tại hoặc đang
trống, rồi đặt vào đó một ứng dụng sẵn sàng để chạy:

- các tệp của starter, với một Go module đặt tên theo thư mục, có thể đổi
  bằng `-module github.com/you/blog`;
- tệp `.env` với một `APP_KEY` mới, 32 byte ngẫu nhiên dùng để mã hóa
  cookie session, và `APP_DEBUG=true`, để hiện chi tiết của lỗi máy chủ
  trên một trang: lỗi, nơi lỗi phát sinh và request;
- các Go module, bằng `go mod tidy`, và các gói của frontend, bằng
  `npm install`;
- mã TypeScript cho các trang và route của ứng dụng, trong
  `resources/js/tug`; để sinh ra mã này, `tug new` build ứng dụng một lần.

Với một ứng dụng có tài khoản người dùng, thêm `-auth`:

```sh
tug new -auth blog
```

Lệnh này đặt thêm một starter thứ hai lên trên starter đầu tiên. Người
dùng đăng ký và xác minh email, đăng nhập, kèm cả mã từ ứng dụng xác thực
khi họ bật tính năng đó, đặt lại mật khẩu đã quên qua liên kết gửi bằng
email, và thay đổi hồ sơ cùng ảnh đại diện, mật khẩu và giao diện trong
phần cài đặt của mình. Người dùng được lưu trong SQLite, ở `app.db`, ảnh
của họ ở `files/`, và frontend dùng Tailwind cùng shadcn/ui. Với
`-postgres` hoặc `-mysql`, người dùng được lưu trong Postgres hoặc MySQL
thay vì SQLite, và `compose.yaml` của ứng dụng chạy cơ sở dữ liệu đó: chạy
`docker compose up -d` trước `tug dev`. Mã nguồn là của chính ứng dụng,
gồm các handler trong `auth.go` và các tệp bên cạnh, cùng SQL của cơ sở dữ
liệu trong `migrations/`, nơi tạo các bảng, và trong các tệp `_db.go`, để
bạn thay đổi theo nhu cầu của ứng dụng ([Migration](../migrations.md)).
Chừng nào `MAIL_HOST` chưa được đặt, email không được gửi đi: khi chạy
`tug dev`, email được giữ trong hộp thư của ứng dụng, tại `/_tug/mail`,
nơi hiện từng email như một chương trình đọc thư, và đầu ra của ứng dụng
có một dòng cho mỗi email, kèm liên kết đến email đó. Xem thêm trong
[Tài khoản](../auth.md).

Để các trang được render trên máy chủ, cho công cụ tìm kiếm và để trang
hiện ra trước khi script của nó chạy, hãy thêm `-ssr`, vào lệnh nào cũng
được: Node, chạy cạnh ứng dụng, sẽ render các trang đó. Xem thêm trong
[Render phía máy chủ](../ssr.md).

Frontend là React, hoặc Vue hay Svelte với `-vue` hay `-svelte`: vẫn là
ứng dụng đó, với cùng mã Go, chỉ khác ở chỗ các trang là component `.vue`
hoặc `.svelte` thay cho `.tsx` của React, và chúng cũng dùng được cùng
`-auth` lẫn `-ssr`. Trang này tiếp tục với starter cơ bản, bằng React.

## Chạy ứng dụng

```sh
tug dev
```

```
tug  │ serving http://localhost:8080, built in 268ms
```

Mở http://localhost:8080. `tug dev` chạy hai máy chủ và hiện đầu ra của
chúng cùng với đầu ra của chính nó, mỗi dòng được gắn nhãn `tug`, `app`
hoặc `vite`:

- **Máy chủ phát triển của Vite**, `npm run dev`, trên `localhost:5173`,
  phục vụ các module của frontend cho trình duyệt.
- **Ứng dụng**, máy chủ Go mà trình duyệt truy cập, được build vào
  `.tug/app`. Khi cổng 8080 đã có chương trình khác dùng, `tug dev` lấy
  cổng trống kế tiếp và báo cho bạn biết. Muốn chọn một cổng cụ thể thì
  dùng `ADDR` hoặc `PORT`, trong `.env` hoặc biến môi trường.

Trang đến từ máy chủ Go, còn script của trang đến từ Vite: trong khi Vite
chạy, plugin trong `vite.config.ts` ghi địa chỉ của Vite vào `public/hot`,
tệp mà máy chủ Go đọc mỗi lần render một trang.

Trong lúc bạn làm việc:

- **Tệp Go, `go.mod` và `go.sum`, cùng các template** (`.html`, `.tmpl`,
  `.gohtml`): tug build lại ứng dụng, sinh lại các kiểu TypeScript nếu
  chúng thay đổi, khởi động lại ứng dụng và tải lại trình duyệt.
- **Build thất bại**: lỗi hiện dưới nhãn `app`, và ứng dụng từ lần build
  thành công gần nhất vẫn chạy cho đến khi một lần lưu sửa được lỗi.
- **Frontend**, trong `resources/`: Vite gửi thay đổi đến trình duyệt, và
  component React được cập nhật tại chỗ, giữ nguyên trạng thái của nó.
- **`.env`**: được đọc khi `tug dev` khởi động. Hãy dừng `tug dev` bằng
  Ctrl-C rồi chạy lại.

Xem chi tiết trong [CLI](../cli.md#tug-dev).

## Trong ứng dụng có gì

```
blog/
├── main.go              máy chủ: các trang, route và handler của nó
├── flash.go             các khóa của dữ liệu flash, được tug gen sinh kiểu
├── main_test.go         các test của nó
├── app.html             HTML mà mọi trang được render vào
├── resources/
│   ├── css/app.css
│   └── js/
│       ├── app.tsx      điểm vào của frontend: tìm component của từng trang
│       ├── Layout.tsx   phần bao quanh mọi trang, kể cả thông báo flash
│       ├── pages/       một component cho mỗi trang: Home.tsx và Error.tsx
│       └── tug/         do tug gen sinh ra, không viết tay: pages.ts, routes.ts
├── public/              được đưa vào binary; Vite build vào public/build
├── vite.config.ts       cấu hình của Vite, và plugin mà tug dev dùng cùng
├── go.mod, package.json, tsconfig.json
├── .env                 do các lệnh tug đọc, không phải ứng dụng; không commit
├── .env.example         như trên nhưng không có khóa, để commit
├── Dockerfile           ứng dụng dưới dạng một binary trên image distroless
└── README.md
```

`main.go` có hai phần. `main` đọc cấu hình của ứng dụng từ biến môi
trường, `APP_KEY` bằng `session.KeysFromEnv`, còn `ADDR`, `PORT` và
`APP_DEBUG` bằng `tug.ConfigFromEnv`, rồi chạy ứng dụng. `newApp` lắp ráp
ứng dụng từ cấu hình đó: các thẻ của Vite, các trang Inertia với
`app.html` bao quanh, session, trang lỗi, middleware, route phục vụ bản
build của frontend dưới `/build/`, và các route riêng của ứng dụng. Các
test gọi `newApp` với cấu hình riêng của chúng.

## Thêm một trang

Một trang gồm một component React trong `resources/js/pages` và mã Go
render nó: một struct chứa props của trang, một `tug.Page` gắn props đó
với component, một handler và một route. Trang ở đây liệt kê các bài viết
của blog. Thêm đoạn sau vào `main.go`, cùng `slices` và `sync` trong phần
import:

```go
// Post là một bài viết trên blog.
type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// posts giữ các bài viết trong bộ nhớ, nên chúng sẽ mất khi ứng dụng dừng.
// Các request được xử lý đồng thời, nên danh sách có một lock.
type posts struct {
	mu   sync.Mutex
	list []Post
}

// PostsIndexProps là props của resources/js/pages/Posts/Index.tsx.
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

và trong `newApp`, bên dưới các route khác:

```go
	p := &posts{}
	app.Get("/posts", p.index).Name("posts.index")
```

`tug.Page[PostsIndexProps]("Posts/Index")` khai báo component
`resources/js/pages/Posts/Index.tsx` và props mà nó nhận: `Render` nhận
một `PostsIndexProps` và không nhận gì khác. Các tag json đặt tên cho
props theo đúng tên mà component nhận, và một danh sách rỗng được gửi đi
dưới dạng `[]`, không bao giờ là `null`. Tên của route, `posts.index`, là
cách cả mã Go lẫn frontend tham chiếu đến route mà không phải viết ra
đường dẫn của nó. Các bài viết được tạo trong `newApp`, nên ứng dụng của
mỗi test bắt đầu mà chưa có bài nào.

Lưu lại, và `tug dev` build ứng dụng rồi sinh lại các kiểu:

```
tug  │ main.go changed
tug  │ wrote resources/js/tug/pages.ts, resources/js/tug/routes.ts
tug  │ serving http://localhost:8080, built in 844ms
```

Giờ `pages.ts` đã có props của trang, bằng TypeScript:

```ts
export interface Post {
  id: number
  title: string
}

export interface PostsIndexProps {
  posts: Post[]
}
```

Component nằm ở `resources/js/pages/Posts/Index.tsx`:

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

`PageProps<'Posts/Index'>` là props riêng của trang cùng với props mà mọi
trang dùng chung, như `appName`, do `newApp` chia sẻ bằng `pages.Share`.
Để liên kết đến trang này từ trang chủ, thêm `Link` vào phần import từ
`@inertiajs/react` của `Home.tsx`, và thêm dòng này bên dưới lời chào:

```tsx
<Link href={route('posts.index')}>Posts</Link>
```

`route()` biết các route có tên của ứng dụng. Một tên không phải tên route
nào, như `route('post.index')`, không qua được kiểm tra kiểu, và một route
có wildcard được gọi mà thiếu giá trị của nó cũng vậy. Giá trị được truyền
theo tên: `route('posts.show', { id: 42 })` là `/posts/42` với route
`/posts/{id}`. Vite không kiểm tra kiểu khi phục vụ; trình soạn thảo của
bạn thì có, `npm run typecheck` và `tug build` cũng vậy.

## Thêm một form

Form này thêm một bài viết. Ở phía Go, đó là một struct chứa những gì form
gửi lên, với các quy tắc cho từng trường trong tag `validate` của nó, và
một handler:

```go
// PostInput là những gì form tạo bài viết mới gửi lên.
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
}

func (p *posts) store(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // quay lại form, kèm các lỗi
	}
	p.mu.Lock()
	p.list = append(p.list, Post{ID: len(p.list) + 1, Title: in.Title})
	p.mu.Unlock()
	c.Flash("success", "Post created")
	return c.RedirectRoute("posts.index")
}
```

và route của nó, trong `newApp`:

```go
	app.Post("/posts", p.store).Name("posts.store")
```

`c.BindValid` điền `in` từ request và kiểm tra nó theo các tag `validate`,
vốn là quy tắc của go-playground/validator. Khi có gì sai, nó trả về các
lỗi, và handler trả chúng về: trình duyệt quay lại form, và trang nhận các
lỗi đó trong prop `errors`, chẳng hạn "title is required" hay "title must
be at most 80 characters". `c.Flash` để lại một thông báo cho trang hiện
ra tiếp theo, và `Layout` của starter sẽ hiện nó. `c.RedirectRoute` chuyển
trình duyệt đến một route có tên, bằng mã 303 sau một POST, để trình duyệt
tiếp tục bằng một GET.

Trong `Index.tsx`, import `Form` cạnh `Head`, và cả `route`:

```tsx
import { Form, Head } from '@inertiajs/react'
import { route } from '../../tug/routes'
```

rồi thêm form bên dưới danh sách:

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

`<Form>` của Inertia gửi các input theo `name` của chúng, cũng là tên json
của các trường trong struct Go, và trao `errors` cho các phần tử con theo
đúng những tên đó. `resetOnSuccess` xóa trống input khi bài viết đã được
thêm. Các class `form` và `error` là CSS của starter.

Trang chủ của starter cũng kiểm tra trường của nó ngay khi người dùng rời
khỏi trường, trước khi form được gửi: input của nó gọi `validate('name')`
khi mất focus, và `BindValid` trả lời request đó, một request
Precognition, bằng các lỗi của trường, rồi handler dừng ở đó. Ở đây cũng
làm được như vậy, với `validate` mà form trao cho các phần tử con, và
`onBlur={() => validate('title')}` trên input. Xem thêm trong
[Form và session](../forms.md): các kiểm tra riêng của handler, error bag,
dữ liệu flash và session.

## Kiểm thử

```sh
go test ./...
```

`main_test.go` của starter kiểm thử ứng dụng theo cách client của Inertia
dùng nó, không cần trình duyệt hay bản build frontend, bằng gói `tugtest`.
`newClient` tạo ứng dụng bằng `newApp`, với một bản build rỗng và một khóa
riêng, rồi trả về một `tugtest.Client`, như một trình duyệt đang mở ứng
dụng. Mỗi lượt truy cập của nó nhận trang dưới dạng JSON, gồm component và
props, và nó giữ cookie session từ lượt này sang lượt sau; chính cookie
này mang lỗi và thông báo flash của form sang trang tiếp theo. Hai test
cho các bài viết, trong `main_test.go`:

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

`c.Post` trả về phản hồi của ứng dụng, ở đây là một chuyển hướng, và
`Follow` đi theo nó đến trang kế tiếp, như trình duyệt vẫn làm: danh sách,
hoặc, với bài viết không có tiêu đề, trang đã gửi form, kèm các lỗi.
`tugtest.Props` đọc props của trang vào `PostsIndexProps` mà `tug.Page` của
trang khai báo, và làm test thất bại nếu đó là trang nào khác.
Xem thêm trong [Kiểm thử](../testing.md).

`npm run typecheck` kiểm tra frontend theo các kiểu mà `tug gen` đã sinh.

## Build ứng dụng

```sh
tug build
```

`tug build` sinh các kiểu, kiểm tra kiểu frontend, build frontend bằng
Vite vào `public/build`, rồi build ứng dụng thành một binary duy nhất,
`./blog`, có frontend bên trong: `main.go` nhúng `public/`. Binary của
starter khoảng 10 MB, và là binary tĩnh, nên chạy được mà không cần cài
thêm gì.

Binary lấy cấu hình từ biến môi trường, không phải từ `.env`:

```sh
APP_KEY=base64:... ADDR=127.0.0.1:8080 ./blog
```

Không có `APP_KEY`, nó sẽ dừng lại và báo rõ điều đó. Một ứng dụng đã
triển khai cần khóa riêng: `tug key` in ra một khóa, có thể gán nguyên văn
cho `APP_KEY`. Ứng dụng lắng nghe trên `ADDR`, hoặc trên `PORT` do các nền
tảng như Cloud Run và Fly.io đặt, nếu không thì trên `:8080`. Đừng đặt
`APP_DEBUG`, để chi tiết của lỗi 500 chỉ nằm trong log.

`Dockerfile` của ứng dụng build frontend bằng Node và binary bằng Go, rồi
chỉ đặt binary lên một image distroless:

```sh
docker build -t blog .
docker run -p 8080:8080 -e APP_KEY=base64:... blog
```

Quá trình build image không chạy `tug`: bước build Vite của nó dùng
`resources/js/tug` có sẵn trong thư mục, thư mục không bị `.gitignore` của
starter bỏ qua, để được commit cùng với mã Go đã sinh ra nó. Xem thêm
trong [Triển khai](../deployment.md): image, biến môi trường và proxy.

## Đọc gì tiếp theo

- [Định tuyến và handler](../routing.md): route, nhóm route, middleware,
  `Ctx`, bind request và lỗi.
- [Trang](../pages.md): props và các loại prop, props dùng chung, chuyển
  hướng, trang lỗi và Vite.
- [Form và session](../forms.md): kiểm tra hợp lệ, Precognition, flash,
  session và CSRF.
- [Tài khoản](../auth.md): starter auth, cùng các gói `auth` và `mail`.
- [TypeScript](../typescript.md): những gì `tug gen` sinh ra từ các kiểu
  Go.
- [Triển khai](../deployment.md): `tug build`, Dockerfile và biến môi
  trường.
- [CLI](../cli.md): từng lệnh, đầy đủ chi tiết.

[`examples/inertia`](../../examples/inertia/main.go) là một ứng dụng lớn
hơn: bài viết được tạo, sửa và xóa, một prop trì hoãn, và một danh sách
tải thêm khi cuộn.
