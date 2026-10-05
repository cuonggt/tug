<?php

namespace App\Providers;

use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    /**
     * Register any application services.
     */
    public function register(): void
    {
        // The page every app of the benchmark serves, read once as the app
        // boots, not for each request: under Octane, once a worker, as a
        // worker keeps its app between requests.
        $this->app->instance('page', json_decode(
            file_get_contents(base_path('../../page/page.json')),
            true,
            flags: JSON_THROW_ON_ERROR,
        ));
    }

    /**
     * Bootstrap any application services.
     */
    public function boot(): void
    {
        //
    }
}
