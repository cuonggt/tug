import { defineConfig } from '@adonisjs/inertia'

const inertiaConfig = defineConfig({
  /**
   * The asset version, fixed at "1", as in every app of the benchmark. Left
   * out, the adapter would hash the Vite manifest when there's one, and say
   * "1" when there's none, as in this app, which has no frontend to build.
   */
  assetsVersion: '1',

  /**
   * Server-side rendering options.
   */
  ssr: {
    /**
     * Toggle SSR mode for Inertia pages.
     */
    enabled: false,
  },
})

export default inertiaConfig
