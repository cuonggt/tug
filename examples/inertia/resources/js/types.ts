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

// Every page also gets the props main.go shares.
interface SharedProps {
  appName: string
}

export interface PostsIndexProps extends SharedProps {
  posts: Post[]
  stats?: Stats // deferred: there once the client has fetched it
}

export interface PostsShowProps extends SharedProps {
  post: Post
}
