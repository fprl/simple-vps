<?php

$path = parse_url($_SERVER["REQUEST_URI"] ?? "/", PHP_URL_PATH) ?: "/";
$file = __DIR__ . $path;

if ($path !== "/" && is_file($file)) {
    return false;
}

if ($path === "/health") {
    header("Content-Type: text/plain");
    echo "ok";
    return;
}

header("Content-Type: application/json");
echo json_encode([
    "app" => "php-plain",
    "status" => "running",
    "secret" => getenv("APP_SECRET") ?: "missing",
    "database_path" => getenv("DATABASE_PATH") ?: null,
], JSON_UNESCAPED_SLASHES) . PHP_EOL;
