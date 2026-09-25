import { Form, Head } from '@inertiajs/react'
import Layout from '../Layout'
import type { PageProps } from '../tug/pages'
import { route } from '../tug/routes'

// Home is the page main.go's home handler renders. Its props, and the
// routes route() knows, are the Go ones: tug gen writes their TypeScript
// into ../tug.
export default function Home({ appName, greeting }: PageProps<'Home'>) {
  return (
    <Layout>
      <Head title="Home" />
      <h1>{appName}</h1>
      <p>{greeting}</p>

      {/* The name is checked by the server as the field is left, and again
          when the form is sent: BindValid in main.go does both. */}
      <Form action={route('hello')} method="post" className="form">
        {({ errors, processing, validate, invalid }) => (
          <>
            <label>
              What's your name?
              <input name="name" aria-invalid={invalid('name')} onBlur={() => validate('name')} />
            </label>
            {errors.name && <p className="error">{errors.name}</p>}
            <button type="submit" disabled={processing}>
              Say hello
            </button>
          </>
        )}
      </Form>

      <p className="hint">
        Change <code>main.go</code> or <code>resources/js/pages/Home.tsx</code>, and the page follows.
      </p>
    </Layout>
  )
}
