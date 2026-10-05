<?php

use App\Http\Controllers\PostController;
use Illuminate\Support\Facades\Route;

Route::get('/posts/{id}', [PostController::class, 'show'])->whereNumber('id');
