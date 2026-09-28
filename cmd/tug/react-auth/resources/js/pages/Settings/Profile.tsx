import { Form, Head, Link } from '@inertiajs/react'
import DeleteAccount from '@/components/delete-account'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import type { PageProps } from '@/tug/pages'
import { route } from '@/tug/routes'

// Profile changes the user's name and email. A new email is mailed a link,
// and isn't verified until it's followed: updateProfile in settings.go.
export default function Profile({ user }: PageProps<'Settings/Profile'>) {
  return (
    <>
      <Head title="Profile" />
      <section className="space-y-6">
        <Heading small title="Profile" description="Your name, and the email we reach you at." />
        <Form action={route('profile.update')} method="patch" options={{ preserveScroll: true }} className="space-y-6">
          {({ errors, processing }) => (
            <>
              <div className="grid gap-2">
                <Label htmlFor="name">Name</Label>
                <Input id="name" name="name" defaultValue={user.name} autoComplete="name" required aria-invalid={!!errors.name} />
                <InputError message={errors.name} />
              </div>
              <div className="grid gap-2">
                <Label htmlFor="email">Email</Label>
                <Input
                  id="email"
                  name="email"
                  type="email"
                  defaultValue={user.email}
                  autoComplete="username"
                  required
                  aria-invalid={!!errors.email}
                />
                <InputError message={errors.email} />
                {!user.emailVerifiedAt && (
                  <p className="text-sm text-muted-foreground">
                    This email isn't verified yet.{' '}
                    <Link
                      href={route('verification.send')}
                      method="post"
                      as="button"
                      preserveScroll
                      className="cursor-pointer text-foreground underline underline-offset-4"
                    >
                      Send the link again
                    </Link>
                  </p>
                )}
              </div>
              <Button type="submit" disabled={processing}>
                Save
              </Button>
            </>
          )}
        </Form>
      </section>
      <DeleteAccount />
    </>
  )
}
