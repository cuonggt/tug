import { Form, Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import type { PageProps } from '../../tug/pages'
import { route } from '../../tug/routes'

// ResetPassword is where the link in a reset mail leads: its token, and
// the email it went to, come in the props.
export default function ResetPassword({ token, email }: PageProps<'Auth/ResetPassword'>) {
  return (
    <Layout>
      <Head title="Choose a new password" />
      <h1>Choose a new password</h1>
      <Form action={route('password.store')} method="post" resetOnError={['password', 'password_confirmation']} className="form">
        {({ errors, processing }) => (
          <>
            <input type="hidden" name="token" value={token} />
            <label>
              Email
              <input name="email" type="email" autoComplete="username" defaultValue={email} required aria-invalid={!!errors.email} />
            </label>
            {errors.email && <p className="error">{errors.email}</p>}

            <label>
              New password
              <input name="password" type="password" autoComplete="new-password" required autoFocus aria-invalid={!!errors.password} />
            </label>
            {errors.password && <p className="error">{errors.password}</p>}

            <label>
              New password again
              <input name="password_confirmation" type="password" autoComplete="new-password" required />
            </label>
            {errors.password_confirmation && <p className="error">{errors.password_confirmation}</p>}

            <button type="submit" disabled={processing}>
              Set the password
            </button>
          </>
        )}
      </Form>
      <p className="hint">
        Has the link stopped working? <Link href={route('password.request')}>Ask for another</Link>.
      </p>
    </Layout>
  )
}
