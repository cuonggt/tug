import { Form, Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import { route } from '../../tug/routes'

// ForgotPassword mails a link that sets a new password. In development,
// without MAIL_HOST in .env, the mail is written to tug dev's terminal.
export default function ForgotPassword() {
  return (
    <Layout>
      <Head title="Forgotten password" />
      <h1>Forgotten your password?</h1>
      <p>Say the email of your account, and we'll mail you a link to choose a new password.</p>
      <Form action={route('password.email')} method="post" resetOnSuccess className="form">
        {({ errors, processing }) => (
          <>
            <label>
              Email
              <input name="email" type="email" autoComplete="email" required autoFocus aria-invalid={!!errors.email} />
            </label>
            {errors.email && <p className="error">{errors.email}</p>}

            <button type="submit" disabled={processing}>
              Mail me a link
            </button>
          </>
        )}
      </Form>
      <p className="hint">
        Remembered it? <Link href={route('login')}>Log in</Link>.
      </p>
    </Layout>
  )
}
