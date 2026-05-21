param(
    [switch]$Build = $true
)

$ErrorActionPreference = "Stop"

$composeArgs = @(
    "-f", "docker-compose.yml",
    "-f", "docker-compose.scale.yml",
    "up", "-d",
    "--scale", "auth=3",
    "--scale", "user=3",
    "--scale", "relation=3",
    "--scale", "group=3",
    "--scale", "msg=3",
    "--scale", "gateway=3",
    "--scale", "message-push=3"
)

if ($Build) {
    $composeArgs += "--build"
}

docker compose @composeArgs

