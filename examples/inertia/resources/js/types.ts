// The props each page gets, as the Go structs in main.go send them. They're
// written by hand to match for now; the roadmap's tug gen writes them.

export interface Post {
  id: number
  title: string
  body: string
  tags: string[]
}

export interface Stats {
  posts: number
  words: number
}

// What every page gets from main.go's pages.Share.
export interface SharedProps {
  appName: string
}

// What main.go's handlers flash with c.Flash.
export interface FlashData {
  success?: string
}

export interface PostsIndexProps {
  posts: { data: Post[] } // a page at a time, as the list scrolls
  stats?: Stats // deferred: there once the client has fetched it
}

export interface PostsShowProps {
  post: Post
}

export interface PostsEditProps {
  post: Post
}

export interface ErrorProps {
  status: number
  message: string
}

// Tells Inertia's own types about the shared props and flash data, so
// usePage() knows them.
declare module '@inertiajs/core' {
  export interface InertiaConfig {
    sharedPageProps: SharedProps
    flashDataType: FlashData
  }
}
