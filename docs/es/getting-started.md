<!-- translated from docs/getting-started.md at 15ea1f8eac10 -->

# Primeros pasos

En esta página vas a crear una aplicación nueva y ejecutarla, añadirle una
página y un formulario, probarlos y compilar la aplicación en un único
binario.

## Lo que necesitas

- Go 1.26 o posterior.
- Node.js 22.12 o posterior, o 20.19 o posterior en Node 20: Vite 8, que
  compila el frontend, necesita una de esas versiones. El Dockerfile de la
  aplicación compila con Node 24.
- npm, que viene con Node. `tug new` instala con él los paquetes de la
  aplicación.

## Instalar tug

```sh
go install github.com/cuonggt/tug/cmd/tug@latest
tug version
```

`go install` pone `tug` en `$(go env GOPATH)/bin`, o en `$GOBIN` si esa
variable está definida, y ese directorio tiene que estar en tu `PATH`.
`tug version` muestra la versión instalada, como `tug v0.39.1`. Las
aplicaciones que crea `tug new` requieren esa versión.

## Crear una aplicación

```sh
tug new blog
cd blog
```

`tug new` crea el directorio `blog`, que no debe existir todavía o debe
estar vacío, y pone en él una aplicación lista para ejecutarse:

- los archivos del kit de inicio, con un módulo de Go que lleva el nombre
  del directorio, salvo que pases `-module github.com/you/blog`;
- un `.env` con una `APP_KEY` nueva, los 32 bytes aleatorios que cifran la
  cookie de sesión, y `APP_DEBUG=true`, que muestra en una página los
  detalles de un error del servidor: el error, de dónde vino y la petición;
- los módulos de Go, con `go mod tidy`, y los paquetes del frontend, con
  `npm install`;
- el TypeScript de las páginas y las rutas de la aplicación, en
  `resources/js/tug`, para lo que compila la aplicación una vez.

Para una aplicación con cuentas, añade `-auth`:

```sh
tug new -auth blog
```

Eso superpone un segundo kit de inicio al primero. Los usuarios se
registran y verifican su correo electrónico; inician sesión, también con un
código de una aplicación de autenticación si lo activan; restablecen una
contraseña olvidada con un enlace que reciben por correo, y cambian en su
configuración su perfil y su foto, su contraseña y la apariencia. Los
usuarios se guardan en SQLite, en `app.db`, sus fotos en `files/`, y el
frontend tiene Tailwind y shadcn/ui. Con `-postgres` o `-mysql`, los
usuarios se guardan en Postgres o MySQL, que pone en marcha el
`compose.yaml` de la aplicación: `docker compose up -d` antes de `tug dev`.
El código es de la propia aplicación, para cambiarlo según sus
necesidades: los handlers, en `auth.go` y los archivos junto a él, y el SQL
de la base de datos, en `migrations/`, que crea sus tablas, y en los
archivos `_db.go` ([Migraciones](../migrations.md)). Mientras `MAIL_HOST`
no esté definida, el correo no se envía: con `tug dev` se guarda en el
buzón de la aplicación, en `/_tug/mail`, que muestra cada correo como lo
haría un cliente de correo, y la salida de la aplicación tiene una línea
por cada uno, con su enlace. [Cuentas](../auth.md) explica el resto.

Para páginas renderizadas en el servidor, para los buscadores y para que
las páginas se vean antes de que se ejecuten sus scripts, añade `-ssr` a
cualquiera de los dos comandos: Node las renderiza, junto a la aplicación.
[Renderizado en el servidor](../ssr.md) explica el resto.

El frontend es React o, con `-vue` o `-svelte`, Vue o Svelte: la misma
aplicación, con el mismo código Go, y sus páginas como componentes `.vue` o
`.svelte` donde las de React son `.tsx`; `-auth` y `-ssr` funcionan también
con ellos. Esta página sigue con el kit de inicio básico, en React.

## Ejecutarla

```sh
tug dev
```

```
tug  │ serving http://localhost:8080, built in 268ms
```

Abre http://localhost:8080. `tug dev` ejecuta dos servidores y muestra su
salida junto con la suya, cada línea marcada con `tug`, `app` o `vite`:

- **El servidor de desarrollo de Vite**, `npm run dev`, en
  `localhost:5173`, que sirve al navegador los módulos del frontend.
- **La aplicación**, el servidor de Go que visita el navegador, compilada
  en `.tug/app`. Si el puerto 8080 está ocupado, `tug dev` toma el
  siguiente libre y lo dice. `ADDR` o `PORT`, en `.env` o en el entorno,
  eligen uno en su lugar.

La página viene del servidor de Go, y sus scripts de Vite: mientras Vite
está en marcha, el plugin de `vite.config.ts` escribe su dirección en
`public/hot`, que el servidor de Go lee cada vez que renderiza una página.

Mientras trabajas:

- **Los archivos de Go, `go.mod` y `go.sum`, y las plantillas** (`.html`,
  `.tmpl`, `.gohtml`): tug vuelve a compilar la aplicación, escribe los
  tipos de TypeScript si han cambiado, reinicia la aplicación y recarga el
  navegador.
- **Una compilación que falla**: sus errores aparecen bajo `app`, y la
  aplicación de la última compilación que funcionó sigue en marcha hasta
  que guardas un arreglo.
- **El frontend**, en `resources/`: Vite envía el cambio al navegador, y un
  componente de React se actualiza sin recargar la página y conserva su
  estado.
- **`.env`**: se lee cuando arranca `tug dev`. Detenlo con Ctrl-C y vuelve
  a arrancarlo.

[La CLI](../cli.md#tug-dev) tiene los detalles.

## Qué hay en la aplicación

```
blog/
├── main.go              el servidor: sus páginas, rutas y handlers
├── flash.go             las claves de los datos flash, a las que tug gen da tipo
├── main_test.go         sus pruebas
├── app.html             el HTML en el que se renderiza cada página
├── resources/
│   ├── css/app.css
│   └── js/
│       ├── app.tsx      el punto de entrada del frontend: encuentra el componente de cada página
│       ├── Layout.tsx   lo que rodea a cada página, también el mensaje flash
│       ├── pages/       un componente por página: Home.tsx y Error.tsx
│       └── tug/         escrito por tug gen, no a mano: pages.ts, routes.ts
├── public/              va dentro del binario; Vite compila en public/build
├── vite.config.ts       la configuración de Vite, y el plugin con el que trabaja tug dev
├── go.mod, package.json, tsconfig.json
├── .env                 lo leen los comandos de tug, no la aplicación; no va al repositorio
├── .env.example         lo mismo sin la clave, para el repositorio
├── Dockerfile           la aplicación como un único binario en una imagen distroless
└── README.md
```

`main.go` tiene dos partes. `main` lee del entorno la configuración de la
aplicación, `APP_KEY` con `session.KeysFromEnv`, y `ADDR`, `PORT` y
`APP_DEBUG` con `tug.ConfigFromEnv`, y ejecuta la aplicación. `newApp` monta
la aplicación a partir de ella: las etiquetas de Vite, las páginas de
Inertia con `app.html` alrededor, las sesiones, la página de error, el
middleware, la ruta que sirve la compilación del frontend bajo `/build/` y
las rutas propias de la aplicación. Las pruebas llaman a `newApp` con una
configuración propia.

## Añadir una página

Una página es un componente de React en `resources/js/pages` y el código Go
que la renderiza: un struct con sus props, un `tug.Page` que las une al
componente, un handler y una ruta. La de este ejemplo lista las entradas
del blog. Añade a `main.go`, con `slices` y `sync` entre sus importaciones:

```go
// Post es una entrada del blog.
type Post struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// posts guarda las entradas en memoria: se pierden al detener la aplicación.
// Las peticiones se atienden a la vez, así que la lista tiene un bloqueo.
type posts struct {
	mu   sync.Mutex
	list []Post
}

// PostsIndexProps son las props de resources/js/pages/Posts/Index.tsx.
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

y en `newApp`, debajo de las demás rutas:

```go
	p := &posts{}
	app.Get("/posts", p.index).Name("posts.index")
```

`tug.Page[PostsIndexProps]("Posts/Index")` declara el componente
`resources/js/pages/Posts/Index.tsx` y las props que recibe: `Render` acepta
un `PostsIndexProps` y nada más. Las etiquetas json dan a las props el
nombre con el que las recibe el componente, y una lista vacía sale como
`[]`, nunca como `null`. El nombre de la ruta, `posts.index`, es como se
refieren a ella tanto el código Go como el frontend sin escribir su URL. Las
entradas se crean en `newApp`, así que la aplicación de cada prueba empieza
sin ninguna.

Guarda, y `tug dev` compila la aplicación y vuelve a escribir sus tipos:

```
tug  │ main.go changed
tug  │ wrote resources/js/tug/pages.ts, resources/js/tug/routes.ts
tug  │ serving http://localhost:8080, built in 844ms
```

Ahora `pages.ts` tiene las props de la página, en TypeScript:

```ts
export interface Post {
  id: number
  title: string
}

export interface PostsIndexProps {
  posts: Post[]
}
```

El componente va en `resources/js/pages/Posts/Index.tsx`:

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

`PageProps<'Posts/Index'>` son las props propias de la página y las que
comparten todas las páginas, como `appName`, que `newApp` comparte con
`pages.Share`. Para enlazarla desde la página de inicio, añade `Link` a lo
que `Home.tsx` importa de `@inertiajs/react`, y esto debajo del saludo:

```tsx
<Link href={route('posts.index')}>Posts</Link>
```

`route()` conoce las rutas con nombre de la aplicación. Un nombre que no es
el de ninguna, como `route('post.index')`, no pasa la comprobación de
tipos, y tampoco una ruta con un comodín llamada sin su valor. Los valores
van por nombre: `route('posts.show', { id: 42 })` es `/posts/42` para una
ruta `/posts/{id}`. Vite no comprueba los tipos mientras sirve; tu editor
sí, y también `npm run typecheck` y `tug build`.

## Añadir un formulario

El formulario añade una entrada. En Go, es un struct con lo que envía el
formulario, con las reglas de cada campo en su etiqueta `validate`, y un
handler:

```go
// PostInput es lo que envía el formulario de nueva entrada.
type PostInput struct {
	Title string `json:"title" validate:"required,max=80"`
}

func (p *posts) store(c *tug.Ctx) error {
	var in PostInput
	if err := c.BindValid(&in); err != nil {
		return err // de vuelta al formulario, con los errores
	}
	p.mu.Lock()
	p.list = append(p.list, Post{ID: len(p.list) + 1, Title: in.Title})
	p.mu.Unlock()
	c.Flash("success", "Post created")
	return c.RedirectRoute("posts.index")
}
```

y su ruta, en `newApp`:

```go
	app.Post("/posts", p.store).Name("posts.store")
```

`c.BindValid` llena `in` con la petición y lo comprueba con las etiquetas
`validate`, que son reglas de go-playground/validator. Si algo está mal,
devuelve los errores, y el handler los devuelve a su vez: el navegador
vuelve al formulario, y la página los recibe en su prop `errors`, como
"title is required" o "title must be at most 80 characters". `c.Flash` deja
un mensaje para la próxima página que se muestre, y el `Layout` del kit de
inicio se encarga de mostrarlo. `c.RedirectRoute` envía al navegador a una
ruta con nombre, con un 303 después de un POST, para que la siga con un
GET.

En `Index.tsx`, importa `Form` junto a `Head`, y `route`:

```tsx
import { Form, Head } from '@inertiajs/react'
import { route } from '../../tug/routes'
```

y añade el formulario debajo de la lista:

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

El `<Form>` de Inertia envía cada campo por su `name`, que es el nombre json
de un campo del struct de Go, y pasa a sus hijos los `errors` con esos
mismos nombres. `resetOnSuccess` vacía el campo en cuanto se añade la
entrada. Las clases `form` y `error` son del CSS del kit de inicio.

La página principal del kit de inicio también comprueba su campo al salir
de él, antes de enviar el formulario: el campo llama a `validate('name')`
cuando pierde el foco, y `BindValid` responde a esa petición, una petición
de Precognition, con los errores del campo, y el handler se detiene ahí.
Aquí funciona igual con el `validate` que el formulario pasa a sus hijos y
`onBlur={() => validate('title')}` en el campo.
[Formularios y sesiones](../forms.md) explica el resto: las comprobaciones
propias del handler, las bolsas de errores, los datos flash y la sesión.

## Probarla

```sh
go test ./...
```

El `main_test.go` del kit de inicio prueba la aplicación como la usa el
cliente de Inertia, sin navegador ni compilación del frontend, con el
paquete `tugtest`. `newClient` crea la aplicación con `newApp`, una
compilación vacía y una clave propia, y devuelve un `tugtest.Client`, un
navegador con la aplicación abierta. Sus visitas reciben cada página como
JSON, su componente y sus props, y conserva la cookie de sesión de una
visita a la siguiente, que es lo que lleva los errores y el mensaje flash
de un formulario a la página que viene después. Dos pruebas para las
entradas, en `main_test.go`:

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

`c.Post` devuelve la respuesta de la aplicación, aquí una redirección, y
`Follow` la sigue hasta la página siguiente, como hace el navegador: la
lista o, para una entrada sin título, la página desde la que se envió el
formulario, con los errores. `tugtest.Props` lee las props de la página en
el `PostsIndexProps` que declara su `tug.Page`, y hace fallar la prueba en
cualquier otra página. [Pruebas](../testing.md) explica el resto.

`npm run typecheck` comprueba el frontend con los tipos que escribió
`tug gen`.

## Compilarla

```sh
tug build
```

`tug build` escribe los tipos, comprueba los tipos del frontend, lo compila
con Vite en `public/build` y compila la aplicación en un único binario,
`./blog`, con el frontend dentro: `main.go` incrusta `public/`. El binario
del kit de inicio ocupa unos 10 MB y es estático, así que funciona sin nada
más instalado.

El binario toma la configuración de su entorno, no de `.env`:

```sh
APP_KEY=base64:... ADDR=127.0.0.1:8080 ./blog
```

Sin una `APP_KEY`, se detiene y lo dice. Una aplicación desplegada necesita
su propia clave: `tug key` muestra una, que `APP_KEY` acepta tal cual. La
aplicación escucha en `ADDR`, o en el `PORT` que definen plataformas como
Cloud Run y Fly.io, o si no, en `:8080`. Deja `APP_DEBUG` sin definir, para
que los detalles de un 500 se queden en el log.

El `Dockerfile` de la aplicación compila el frontend con Node y el binario
con Go, y pone solo el binario en una imagen distroless:

```sh
docker build -t blog .
docker run -p 8080:8080 -e APP_KEY=base64:... blog
```

La compilación de la imagen no ejecuta `tug`: su compilación con Vite usa el
`resources/js/tug` del directorio, que el `.gitignore` del kit de inicio no
excluye, para que vaya al repositorio junto con el código Go a partir del
cual se escribe. [Despliegue](../deployment.md) explica el resto: la
imagen, el entorno y los proxies.

## Siguientes pasos

- [Rutas y handlers](../routing.md): rutas, grupos, middleware, `Ctx`,
  binding y errores.
- [Páginas](../pages.md): props y tipos de props, props compartidas,
  redirecciones, páginas de error y Vite.
- [Formularios y sesiones](../forms.md): validación, Precognition, mensajes
  flash, sesiones y CSRF.
- [Cuentas](../auth.md): el kit de inicio de autenticación y los paquetes
  `auth` y `mail`.
- [TypeScript](../typescript.md): lo que `tug gen` escribe a partir de los
  tipos de Go.
- [Despliegue](../deployment.md): `tug build`, el Dockerfile y el entorno.
- [La CLI](../cli.md): cada comando en detalle.

[`examples/inertia`](../../examples/inertia/main.go) es una aplicación más
grande: entradas que se crean, editan y eliminan, una prop diferida y una
lista que carga más a medida que se desplaza.
