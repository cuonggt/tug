// What the handlers flash with c.Flash, for usePage().flash: a message for
// a toast after a form, and the recovery codes shown once, as two-factor
// logins are turned on. The props of the pages, and the routes, are in
// ./tug, written by tug gen.
declare module '@inertiajs/core' {
  export interface InertiaConfig {
    flashDataType: { success?: string; error?: string; recoveryCodes?: string[] }
  }
}

export {}
