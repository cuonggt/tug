import { Form, Head, Link } from '@inertiajs/react'
import Layout from '../../Layout'
import { route } from '../../tug/routes'

// Register checks each field with the server as it's left, as BindValid in
// auth.go answers: a taken email shows before the form is sent.
export default function Register() {
  return (
    <Layout>
      <Head title="Register" />
      <h1>Register</h1>
      <Form
        action={route('register.store')}
        method="post"
        resetOnError={['password', 'password_confirmation']}
        validationTimeout={300}
        className="form"
      >
        {({ errors, processing, validate, invalid }) => (
          <>
            <label>
              Name
              <input name="name" autoComplete="name" required autoFocus aria-invalid={invalid('name')} onBlur={() => validate('name')} />
            </label>
            {errors.name && <p className="error">{errors.name}</p>}

            <label>
              Email
              <input name="email" type="email" autoComplete="email" required aria-invalid={invalid('email')} onBlur={() => validate('email')} />
            </label>
            {errors.email && <p className="error">{errors.email}</p>}

            <label>
              Password
              <input
                name="password"
                type="password"
                autoComplete="new-password"
                required
                aria-invalid={invalid('password')}
                onBlur={() => validate('password')}
              />
            </label>
            {errors.password && <p className="error">{errors.password}</p>}

            <label>
              Password again
              <input
                name="password_confirmation"
                type="password"
                autoComplete="new-password"
                required
                aria-invalid={invalid('password_confirmation')}
                onBlur={() => validate('password_confirmation')}
              />
            </label>
            {errors.password_confirmation && <p className="error">{errors.password_confirmation}</p>}

            <button type="submit" disabled={processing}>
              Register
            </button>
          </>
        )}
      </Form>
      <p className="hint">
        Have an account? <Link href={route('login')}>Log in</Link>.
      </p>
    </Layout>
  )
}
