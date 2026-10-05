class PostsController < ApplicationController
  # The page's props, which every app in the benchmark reads from the same
  # file. Read once, as production eager loads this class at boot, and frozen,
  # as every request's threads share it.
  PAGE = JSON.load_file(Rails.root.join("../../page/page.json"), freeze: true)

  def show
    render inertia: "Posts/Show", props: {
      # merge keeps the id where page.json has it, the post's first key.
      post: PAGE["post"].merge("id" => params[:id].to_i),
      comments: PAGE["comments"]
    }
  end
end
