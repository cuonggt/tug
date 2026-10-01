import { Form, Head, Link, router } from '@inertiajs/react'
import DeleteAccount from '@/components/delete-account'
import Heading from '@/components/heading'
import InputError from '@/components/input-error'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { initials } from '@/lib/utils'
import type { PageProps } from '@/tug/pages'
import { form, route, type Inputs } from '@/tug/routes'

// Profile changes the user's name and email, and their photo. A new email
// is mailed a link, and isn't verified until it's followed: updateProfile
// in settings.go. The photo goes up as the form's file, with its progress
// shown, and replaces the one before: updatePhoto in photos.go.
export default function Profile({ user }: PageProps<'Settings/Profile'>) {
  return (
    <>
      <Head title="Profile" />
      <section className="space-y-6">
        <Heading small title="Profile" description="Your name, and the email we reach you at." />
        <Form<Inputs['profile.update']> action={form('profile.update')} options={{ preserveScroll: true }} className="space-y-6">
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
      <section className="space-y-6">
        <Heading small title="Photo" description="Shown in place of your initials: a PNG, JPEG or WebP, of 2 MB at most." />
        <div className="flex items-start gap-6">
          <Avatar key={user.photo} className="size-16">
            {user.photo && <AvatarImage src={user.photo} alt="Your photo" />}
            <AvatarFallback className="text-lg font-medium">{initials(user.name)}</AvatarFallback>
          </Avatar>
          <Form<Inputs['profile.photo.update']>
            action={form('profile.photo.update')}
            options={{ preserveScroll: true }}
            resetOnSuccess
            className="grid flex-1 gap-2"
          >
            {({ errors, processing, progress }) => (
              <>
                <Label htmlFor="photo">Choose a photo</Label>
                <Input
                  id="photo"
                  name="photo"
                  type="file"
                  accept="image/png,image/jpeg,image/webp"
                  required
                  aria-invalid={!!errors.photo}
                />
                <InputError message={errors.photo} />
                {progress && (
                  <progress value={progress.percentage} max={100} aria-label="Uploading" className="w-full accent-primary" />
                )}
                <div className="flex gap-2">
                  <Button type="submit" disabled={processing}>
                    Upload
                  </Button>
                  {user.photo && (
                    <Button
                      type="button"
                      variant="outline"
                      onClick={() => router.delete(route('profile.photo.destroy'), { preserveScroll: true })}
                    >
                      Remove
                    </Button>
                  )}
                </div>
              </>
            )}
          </Form>
        </div>
      </section>
      <DeleteAccount />
    </>
  )
}
