$containerName = "price_tracker_bot"

# Move to the script's directory
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Definition
Set-Location $scriptPath

# Navigate to project root (assuming 'deployment' is a subfolder)
Set-Location ..

# Check if the container is running
$runningContainer = docker ps -q -f "name=$containerName"
$existingContainer = docker ps -a -q -f "name=$containerName"

if ($runningContainer) {
    Write-Host "Container $containerName is already running."
} elseif ($existingContainer) {
    Write-Host "Starting the existing container: $containerName"
    docker start $containerName
} else {
    Write-Host "No existing container found. Creating and starting a new container: $containerName"
    docker run --name $containerName --env-file .env -p 7080:8080 $containerName
}

Read-Host -Prompt "Press Enter to exit"
