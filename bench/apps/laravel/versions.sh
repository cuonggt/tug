#!/usr/bin/env bash
# PHP and Swoole as they run, and the packages as composer.lock pins them.
set -euo pipefail
php -r '
$lock = json_decode(file_get_contents("composer.lock"), true, flags: JSON_THROW_ON_ERROR);
foreach ($lock["packages"] as $package) {
    $versions[$package["name"]] = ltrim($package["version"], "v");
}
echo "PHP ", PHP_VERSION, "\n";
echo "Laravel ", $versions["laravel/framework"], "\n";
echo "inertia-laravel ", $versions["inertiajs/inertia-laravel"], "\n";
echo "Octane ", $versions["laravel/octane"], "\n";
echo "Swoole ", phpversion("swoole"), "\n";
'
