<!-- translated from docs/README.md at ba93a85f29e8 -->

# La guía de tug

tug es un framework web para aplicaciones en Go cuyo frontend es
[Inertia.js](https://inertiajs.com): los handlers de Go renderizan páginas
de React, Vue o Svelte con props, sin una API de por medio, y toda la
aplicación se distribuye como un único binario. Esta guía va de una
aplicación nueva a una desplegada. La documentación de cada paquete, en
[pkg.go.dev](https://pkg.go.dev/github.com/cuonggt/tug), tiene el resto de
los detalles.

1. [Primeros pasos](getting-started.md): instalar tug, crear una aplicación,
   ejecutarla y añadirle una página y un formulario.
2. [Rutas y handlers](../routing.md): la App, las rutas, los enlaces
   absolutos y firmados, los grupos, los middleware, entre ellos los de las
   cabeceras de seguridad, la ruta que respondió, la dirección del cliente
   detrás de un proxy, `Ctx`, archivos, descargas, streams y eventos, el
   binding de peticiones y los errores.
3. [Páginas](../pages.md): las páginas de Inertia y sus props, la plantilla
   raíz y su nonce, las props que se calculan más tarde, las props
   compartidas, las redirecciones, las descargas y los eventos en una
   página, las páginas de error, Vite y las DevTools de Inertia.
4. [Renderizado en el servidor](../ssr.md): páginas renderizadas en el
   servidor para una primera visita, por Node junto a la aplicación, y el
   paquete `ssr`.
5. [Formularios y sesiones](../forms.md): la validación, reglas propias de
   la aplicación, formularios que comprueban cada campo al salir de él, los
   mensajes flash, las sesiones y CSRF.
6. [Idiomas](../languages.md): lo que dicen tug y la aplicación, en el
   idioma de la petición, a partir de un archivo por idioma, el paquete
   `lang` y `tug lang`.
7. [Archivos](../files.md): los archivos subidos, comprobados por su tamaño
   y por lo que son, guardados en el disco de la aplicación o en S3 con el
   paquete `storage`, y los enlaces a ellos, públicos o firmados.
8. [Cuentas](../auth.md): el kit de inicio de autenticación, en SQLite,
   Postgres o MySQL, sus tokens de API, administradores y notificaciones, y
   los paquetes `auth` y `mail`, con copias, archivos adjuntos y un enlace
   para darse de baja con un clic.
9. [Migraciones](../migrations.md): las tablas de la base de datos, creadas
   y modificadas por archivos SQL que se ejecutan una vez cada uno, en
   orden, al arrancar la aplicación y con su comando `migrate`, con
   `tug migrate new` para crear el siguiente, y el paquete `migrate`.
10. [Autorización](../authorization.md): lo que un usuario puede hacer con
    algo, las habilidades del paquete `auth`, una negativa en forma de 403
    que dice por qué, y lo que puede hacer el usuario de una página, en sus
    props.
11. [Cifrado](../encryption.md): la clave de la aplicación, lo que tug cifra
    y firma con ella, el paquete `crypt`, para los valores propios de la
    aplicación, y la rotación de la clave.
12. [Tareas en segundo plano](../jobs.md): el paquete `queue`, para el
    trabajo que dura más que la petición: se vuelve a ejecutar cuando
    falla, se ejecuta según un horario, también en una zona horaria, queda
    en cola una sola vez por muchas veces que se pida, o como mucho tantas
    a la vez o por segundo, en todas las instancias, y avisa cuando ha
    fallado definitivamente, y cuenta cómo ha ido cada ejecución.
13. [Caché](../cache.md): el paquete `cache`, para lo que es lento de
    calcular, guardado donde lo encuentran todas las instancias de la
    aplicación, y bloqueos para lo que no debe ejecutarse dos veces a la
    vez.
14. [Difusión de eventos](../broadcasting.md): el paquete `broadcast`,
    eventos en canales llevados por la base de datos de la aplicación hasta
    las páginas abiertas en todas las instancias, que recargan lo que
    cambió.
15. [TypeScript](../typescript.md): los tipos que `tug gen` escribe a partir
    del código Go.
16. [Pruebas](../testing.md): el paquete `tugtest`, el cliente de Inertia
    para las pruebas en Go de las páginas, los formularios, las subidas de
    archivos y los inicios de sesión de una aplicación, `mailtest`, para su
    correo, y sus eventos.
17. [Despliegue](../deployment.md): un único binario, el Dockerfile, el
    entorno, las cabeceras que dicen qué puede hacer un navegador con las
    páginas de la aplicación, entre ellas una Content-Security-Policy, los
    logs y las métricas.
18. [La CLI](../cli.md): `tug new`, `tug dev`, `tug gen`, `tug lang`,
    `tug migrate`, `tug build` y `tug key`, en detalle.

[La hoja de ruta](../roadmap.md) cuenta cómo se construyó tug, las
decisiones que hay detrás y lo que viene después, y
[los benchmarks](../benchmarks.md), lo que cuesta tug en cada petición, lo
que necesita para funcionar una aplicación hecha con él y cómo se midió.
