import { Form } from '@inertiajs/react'
import type { Post } from './tug/pages'

interface PostFormProps {
  action: string
  method: 'post' | 'put'
  post?: Post
  submit: string
}

// PostForm writes a post. Each field is checked by the server as it's
// left, with a Precognition request that the handler's BindValid answers,
// and again, all together, when the form is sent.
export default function PostForm({ action, method, post, submit }: PostFormProps) {
  return (
    <Form action={action} method={method} validationTimeout={300} className="post-form">
      {({ errors, processing, validate, invalid }) => (
        <>
          <label>
            Title
            <input name="title" defaultValue={post?.title} aria-invalid={invalid('title')} onBlur={() => validate('title')} />
          </label>
          {errors.title && <p className="error">{errors.title}</p>}

          <label>
            Body
            <textarea name="body" rows={5} defaultValue={post?.body} aria-invalid={invalid('body')} onBlur={() => validate('body')} />
          </label>
          {errors.body && <p className="error">{errors.body}</p>}

          <label>
            Tags, comma separated
            <input name="tags" defaultValue={post?.tags.join(', ')} aria-invalid={invalid('tags')} onBlur={() => validate('tags')} />
          </label>
          {errors.tags && <p className="error">{errors.tags}</p>}

          <button type="submit" disabled={processing}>
            {submit}
          </button>
        </>
      )}
    </Form>
  )
}
