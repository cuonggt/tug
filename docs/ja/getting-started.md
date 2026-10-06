<!-- translated from docs/getting-started.md at 9c124cf969af -->

# はじめに

このページでは、新しいアプリを作って実行し、ページとフォームを追加してテストし、アプリを 1 つのバイナリにビルドします。

## 必要なもの

- Go 1.26 以降。
- Node.js 22.12 以降、または Node 20 なら 20.19 以降。フロントエンドをビルドする Vite 8 が、このどちらかを必要とします。アプリの Dockerfile は Node 24 でビルドします。
- npm（Node に付属しています）。`tug new` はこれを使ってアプリのパッケージをインストールします。

## tug をインストールする

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug version
```

`go install` は `tug` を `$(go env GOPATH)/bin` に置きます。`$GOBIN` が設定されていれば、そちらに置きます。そのディレクトリが `PATH` に含まれている必要があります。`tug version` は、`tug v0.39.0` のように、インストールされたバージョンを表示します。`tug new` が作るアプリは、このバージョンに依存します。

## アプリを作る

```sh
tug new blog
cd blog
```

`tug new` は `blog` ディレクトリを作り、すぐに実行できるアプリをそこに置きます。このディレクトリは、まだ存在しないか、空でなければなりません。置かれるのは次のものです。

- スターターのファイル。Go モジュールの名前はディレクトリ名になり、`-module github.com/you/blog` で変更できます。
- `.env`。新しく作られた `APP_KEY`（セッション Cookie を暗号化する 32 バイトの乱数）と、`APP_DEBUG=true` が書かれています。`APP_DEBUG=true` だと、サーバーエラーの詳細（エラー、その発生元、リクエスト）がページに表示されます。
- `go mod tidy` で揃えた Go モジュールと、`npm install` でインストールしたフロントエンドのパッケージ。
- `resources/js/tug` にある、アプリのページとルートの TypeScript。これを書き出すために、`tug new` はアプリを一度ビルドします。

アカウント機能のあるアプリにするには、`-auth` を付けます。

```sh
tug new -auth blog
```

これで、1 つ目のスターターの上に 2 つ目のスターターが重なります。ユーザーは登録してメールアドレスを確認し、ログインします。認証アプリのコードを使う設定を有効にしたユーザーは、ログイン時にそのコードも入力します。忘れたパスワードはメールで届くリンクからリセットでき、プロフィールと写真、パスワード、外観は設定画面で変更できます。ユーザーは SQLite の `app.db` に、写真は `files/` に保存され、フロントエンドには Tailwind と shadcn/ui が入っています。`-postgres` か `-mysql` を付けると、ユーザーは代わりに Postgres か MySQL に保存されます。そのデータベースはアプリの `compose.yaml` で起動するので、`tug dev` の前に `docker compose up -d` を実行します。コードはアプリ自身のものです。ハンドラーは `auth.go` とその隣のファイルに、データベースの SQL はテーブルを作る `migrations/` と `_db.go` ファイルにあり、アプリの必要に応じて変更できます（[マイグレーション](../migrations.md)）。`MAIL_HOST` を設定するまで、メールは送信されません。`tug dev` で動かしている間は、メールはアプリのメールボックス（`/_tug/mail`）に保管されます。メールボックスは各メールをメールソフトと同じように表示し、アプリの出力にはメールごとに、そのリンクを含む行が出ます。詳しくは[アカウント](../auth.md)を参照してください。

検索エンジンのために、またスクリプトが動く前からページを表示するために、ページをサーバーでレンダリングするなら、どちらのコマンドにも `-ssr` を付けます。レンダリングは、アプリと並んで動く Node が行います。詳しくは[サーバーサイドレンダリング](../ssr.md)を参照してください。

フロントエンドは React ですが、`-vue` か `-svelte` を付けると Vue か Svelte になります。アプリも Go のコードも同じで、React では `.tsx` のページが `.vue` や `.svelte` のコンポーネントになります。`-auth` や `-ssr` とも組み合わせられます。このページでは、React の基本のスターターで進めます。

## 実行する

```sh
tug dev
```

```
tug  │ serving http://localhost:8080, built in 268ms
```

http://localhost:8080 を開きます。`tug dev` は 2 つのサーバーを動かし、その出力を自身の出力とあわせて表示します。各行には `tug`、`app`、`vite` のいずれかのラベルが付きます。

- **Vite の開発サーバー**（`npm run dev`）。`localhost:5173` で動き、フロントエンドのモジュールをブラウザーに配信します。
- **アプリ**。ブラウザーがアクセスする Go のサーバーで、`.tug/app` にビルドされます。ポート 8080 が使われていれば、`tug dev` は次に空いているポートを使い、そのことを表示します。`.env` か環境変数で `ADDR` または `PORT` を設定すれば、代わりにそのポートを使います。

ページは Go のサーバーから、そのスクリプトは Vite から届きます。Vite の実行中は、`vite.config.ts` にあるプラグインが Vite のアドレスを `public/hot` に書き込み、Go のサーバーはページをレンダリングするたびにそれを読みます。

作業中は、変更に応じて次のことが起こります。

- **Go のファイル、`go.mod` と `go.sum`、テンプレート**（`.html`、`.tmpl`、`.gohtml`）：tug はアプリをビルドし直し、TypeScript の型が変わっていれば書き出し、アプリを再起動して、ブラウザーを再読み込みします。
- **ビルドが失敗したとき**：エラーは `app` のラベルが付いた行に表示され、保存して直すまでは、最後に成功したビルドのアプリが動き続けます。
- **フロントエンド**（`resources/` の下）：Vite が変更をブラウザーに送り、React コンポーネントは状態を保ったまま、その場で更新されます。
- **`.env`**：`tug dev` の起動時に読み込まれます。Ctrl-C で止めて、もう一度起動してください。

詳しくは [CLI](../cli.md#tug-dev) を参照してください。

## アプリの中身

```
blog/
├── main.go              サーバー：ページ、ルート、ハンドラー
├── flash.go             フラッシュデータのキー。tug gen が型を付ける
├── main_test.go         テスト
├── app.html             どのページもこの中にレンダリングされる HTML
├── resources/
│   ├── css/app.css
│   └── js/
│       ├── app.tsx      フロントエンドのエントリポイント：各ページのコンポーネントを探す
│       ├── Layout.tsx   全ページを囲む部分。フラッシュメッセージもここで表示
│       ├── pages/       ページごとのコンポーネント：Home.tsx と Error.tsx
│       └── tug/         手ではなく tug gen が書く：pages.ts、routes.ts
├── public/              バイナリに入る。Vite は public/build にビルドする
├── vite.config.ts       Vite の設定と、tug dev と連携するプラグイン
├── go.mod, package.json, tsconfig.json
├── .env                 アプリではなく tug のコマンドが読む。コミットしない
├── .env.example         キーを除いた同じもの。こちらをコミットする
├── Dockerfile           distroless イメージ上の、バイナリ 1 つのアプリ
└── README.md
```

`main.go` は 2 つの部分からなります。`main` は、アプリの設定を環境変数から読み込み（`APP_KEY` は `session.KeysFromEnv` で、`ADDR`、`PORT`、`APP_DEBUG` は `tug.ConfigFromEnv` で）、アプリを実行します。`newApp` は、その設定からアプリを組み立てます。組み立てるのは、Vite のタグ、`app.html` で囲まれた Inertia のページ、セッション、エラーページ、ミドルウェア、`/build/` の下でフロントエンドのビルドを配信するルート、そしてアプリ自身のルートです。テストは独自の設定で `newApp` を呼び出します。

## ページを追加する

ページは、`resources/js/pages` にある React コンポーネントと、それをレンダリングする Go のコードからなります。Go 側は、props の構造体、それをコンポーネントに結び付ける `tug.Page`、ハンドラー、ルートです。ここで追加するページは、ブログの投稿を一覧表示します。`main.go` の import に `slices` と `sync` を加えて、次のコードを追加します。

```go
// Post はブログの投稿です。
type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// posts は投稿をメモリに保持するので、アプリが止まると投稿は消えます。
// リクエストは並行して処理されるので、リストにはロックを付けます。
type posts struct {
	mu   sync.Mutex
	list []Post
}

// PostsIndexProps は resources/js/pages/Posts/Index.tsx の props です。
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

`newApp` では、ほかのルートの下に次を追加します。

```go
	p := &posts{}
	app.Get("/posts", p.index).Name("posts.index")
```

`tug.Page[PostsIndexProps]("Posts/Index")` は、コンポーネント `resources/js/pages/Posts/Index.tsx` と、それが受け取る props を宣言します。`Render` が受け取るのは `PostsIndexProps` だけです。json タグは、コンポーネントが受け取るときの props の名前を決めます。空のリストは `[]` として送られ、`null` になることはありません。ルート名 `posts.index` を使えば、Go からもフロントエンドからも、パスを書かずにそのルートを参照できます。投稿は `newApp` の中で作られるので、テストごとのアプリは投稿がない状態から始まります。

保存すると、`tug dev` がアプリをビルドし、型を書き出し直します。

```
tug  │ main.go changed
tug  │ wrote resources/js/tug/pages.ts, resources/js/tug/routes.ts
tug  │ serving http://localhost:8080, built in 844ms
```

これで `pages.ts` には、ページの props が TypeScript で書かれています。

```ts
export interface Post {
  id: number
  title: string
}

export interface PostsIndexProps {
  posts: Post[]
}
```

コンポーネントは `resources/js/pages/Posts/Index.tsx` に置きます。

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

`PageProps<'Posts/Index'>` は、このページ自身の props と、全ページが共有する props を合わせたものです。共有する props には、`newApp` が `pages.Share` で共有する `appName` などがあります。トップページからこのページにリンクするには、`Home.tsx` で `@inertiajs/react` から import しているものに `Link` を加え、挨拶の下に次を追加します。

```tsx
<Link href={route('posts.index')}>Posts</Link>
```

`route()` はアプリの名前付きルートを知っています。`route('post.index')` のように存在しない名前は、型チェックを通りません。ワイルドカードのあるルートを、その値なしで呼び出した場合も同じです。値は名前で渡します。`/posts/{id}` というルートなら、`route('posts.show', { id: 42 })` は `/posts/42` になります。Vite は配信するときに型をチェックしません。型をチェックするのはエディターと、`npm run typecheck`、`tug build` です。

## フォームを追加する

このフォームは投稿を追加します。Go 側では、フォームが送信する内容の構造体（各フィールドのルールは `validate` タグに書きます）と、ハンドラーを用意します。

```go
// PostInput は新しい投稿のフォームが送信する内容です。
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
}

func (p *posts) store(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // エラーとともにフォームへ戻る
	}
	p.mu.Lock()
	p.list = append(p.list, Post{ID: len(p.list) + 1, Title: in.Title})
	p.mu.Unlock()
	c.Flash("success", "Post created")
	return c.RedirectRoute("posts.index")
}
```

そのルートを `newApp` に追加します。

```go
	app.Post("/posts", p.store).Name("posts.store")
```

`c.BindValid` は、リクエストから `in` を埋め、`validate` タグに照らして検証します。タグに書くのは go-playground/validator のルールです。問題があればエラーを返し、ハンドラーはそれをそのまま返します。するとブラウザーはフォームに戻り、ページは「title is required」や「title must be at most 80 characters」のようなエラーを `errors` という props で受け取ります。`c.Flash` は次に表示されるページのためにメッセージを残し、スターターの `Layout` がそれを表示します。`c.RedirectRoute` はブラウザーを名前付きルートへ送ります。POST の後は 303 で送るので、ブラウザーは GET でその先を開きます。

`Index.tsx` では、`Head` と並べて `Form` を、そして `route` を import します。

```tsx
import { Form, Head } from '@inertiajs/react'
import { route } from '../../tug/routes'
```

リストの下にフォームを追加します。

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

Inertia の `<Form>` は、入力を `name` ごとに送信します。この名前は Go の構造体のフィールドの json 名です。`<Form>` は、同じ名前で `errors` を子要素に渡します。`resetOnSuccess` は、投稿が追加されると入力欄を空にします。`form` と `error` のクラスはスターターの CSS にあります。

スターターのトップページは、フォームが送信される前にも、フォーカスが外れた時点でフィールドを検証します。入力欄はフォーカスを失うと `validate('name')` を呼び出し、`BindValid` はその Precognition リクエストにフィールドのエラーで応答して、ハンドラーはそこで止まります。ここでも、フォームの子要素で受け取る `validate` を使い、入力欄に `onBlur={() => validate('title')}` を付ければ、同じように動きます。ハンドラー独自のチェック、エラーバッグ、フラッシュデータ、セッションなど、詳しくは[フォームとセッション](../forms.md)を参照してください。

## テストする

```sh
go test ./...
```

スターターの `main_test.go` は、`tugtest` パッケージを使い、ブラウザーもフロントエンドのビルドもなしで、Inertia のクライアントと同じやり方でアプリをテストします。`newClient` は、空のビルドと専用のキーを使って `newApp` でアプリを作り、`tugtest.Client` を返します。これはアプリを開いたブラウザーにあたります。このクライアントの訪問では、各ページをコンポーネントと props からなる JSON として受け取り、セッション Cookie は訪問から次の訪問へと保持されます。フォームのエラーやフラッシュメッセージを次のページへ運ぶのは、この Cookie です。投稿のテストを 2 つ、`main_test.go` に書きます。

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

`c.Post` はアプリの応答（ここではリダイレクト）を返し、`Follow` はブラウザーと同じようにそれをたどって次のページに移ります。移る先は一覧です。タイトルのない投稿なら、フォームを送信したページにエラーとともに戻ります。`tugtest.Props` は、ページの props を、その `tug.Page` が宣言する `PostsIndexProps` に読み込みます。ほかのページなら、テストを失敗させます。詳しくは[テスト](../testing.md)を参照してください。

`npm run typecheck` は、`tug gen` が書き出した型に照らしてフロントエンドをチェックします。

## ビルドする

```sh
tug build
```

`tug build` は型を書き出し、フロントエンドを型チェックして Vite で `public/build` にビルドし、アプリを 1 つのバイナリ `./blog` にビルドします。`main.go` が `public/` を埋め込むので、フロントエンドはバイナリの中に入ります。スターターのバイナリは約 10 MB の静的バイナリなので、ほかに何もインストールせずに動きます。

バイナリは設定を `.env` からではなく、環境変数から読みます。

```sh
APP_KEY=base64:... ADDR=127.0.0.1:8080 ./blog
```

`APP_KEY` がなければ、そのことを表示して停止します。デプロイするアプリには専用のキーが必要です。`tug key` がキーを 1 つ出力するので、それをそのまま `APP_KEY` に設定します。アプリは `ADDR` で待ち受けます。なければ、Cloud Run や Fly.io などのプラットフォームが設定する `PORT` で、それもなければ `:8080` で待ち受けます。500 エラーの詳細がログにとどまるよう、`APP_DEBUG` は設定しないでおきます。

アプリの `Dockerfile` は、フロントエンドを Node で、バイナリを Go でビルドし、バイナリだけを distroless イメージに載せます。

```sh
docker build -t blog .
docker run -p 8080:8080 -e APP_KEY=base64:... blog
```

イメージのビルドでは `tug` を実行しません。その中の Vite のビルドは、ディレクトリにある `resources/js/tug` を使います。スターターの `.gitignore` はこのディレクトリを除外しないので、書き出し元の Go のコードと一緒にコミットします。イメージ、環境変数、プロキシなど、詳しくは[デプロイ](../deployment.md)を参照してください。

## 次に読むもの

- [ルーティングとハンドラー](../routing.md)：ルート、グループ、ミドルウェア、`Ctx`、バインド、エラー。
- [ページ](../pages.md)：props と props の種類、共有 props、リダイレクト、エラーページ、Vite。
- [フォームとセッション](../forms.md)：バリデーション、Precognition、フラッシュ、セッション、CSRF。
- [アカウント](../auth.md)：認証スターターと、`auth` パッケージ、`mail` パッケージ。
- [TypeScript](../typescript.md)：`tug gen` が Go の型から書き出すもの。
- [デプロイ](../deployment.md)：`tug build`、Dockerfile、環境変数。
- [CLI](../cli.md)：各コマンドの詳細。

[`examples/inertia`](../../examples/inertia/main.go) はもっと大きなアプリです。投稿の作成・編集・削除、遅延 props、スクロールに合わせて続きを読み込むリストがあります。
