// What the handlers flash with c.Flash, for usePage().flash. The props of
// the pages, and the routes, are in ./tug, written by tug gen.
declare module '@inertiajs/core' {
  export interface InertiaConfig {
    flashDataType: { success?: string }
  }
}

export {}
