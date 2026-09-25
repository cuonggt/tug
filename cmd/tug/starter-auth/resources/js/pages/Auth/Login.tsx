import { Form, Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import { route } from '../../tug/routes'

export default function Login() {
  return (
    <Layout>
      <Head title="Log in" />
      <h1>Log in</h1>
      <Form action={route('login.store')} method="post" resetOnError={['password']} className="form">
        {({ errors, processing }) => (
          <>
            <label>
              Email
              <input name="email" type="email" autoComplete="username" required autoFocus aria-invalid={!!errors.email} />
            </label>
            {errors.email && <p className="error">{errors.email}</p>}

            <label>
              Password
              <input name="password" type="password" autoComplete="current-password" required />
            </label>
            {errors.password && <p className="error">{errors.password}</p>}

            <button type="submit" disabled={processing}>
              Log in
            </button>
          </>
        )}
      </Form>
      <p className="hint">
        <Link href={route('password.request')}>Forgotten your password?</Link> No account yet?{' '}
        <Link href={route('register')}>Register</Link>.
      </p>
    </Layout>
  )
}
