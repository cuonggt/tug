// What main.go's handlers flash with c.Flash, for usePage().flash. The
// pages' props, the shared props and the routes are in ./tug, which tug gen
// writes from the Go.
declare module '@inertiajs/core' {
  export interface InertiaConfig {
    flashDataType: { success?: string }
  }
}

export {}
