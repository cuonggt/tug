<?php

namespace App\Http\Controllers;

use Inertia\Inertia;
use Inertia\Response;

class PostController extends Controller
{
    /**
     * The post with the route's ID, and its comments: page.json's.
     */
    public function show(int $id): Response
    {
        ['post' => $post, 'comments' => $comments] = app('page');

        // page.json's post has its id first, so setting it keeps it there.
        $post['id'] = $id;

        return Inertia::render('Posts/Show', [
            'post' => $post,
            'comments' => $comments,
        ]);
    }
}
