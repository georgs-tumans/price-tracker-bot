$containerName = "price_tracker_bot"

# Move to the script's directory
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Definition
Set-Location $scriptPath

# Navigate to project root (assuming 'deployment' is a subfolder)
Set-Location ..

# Check if the container is running
$runningContainer = docker ps -q -f "name=$containerName"

if ($runningContainer) {
    Write-Host "Stopping running container: $containerName"
    docker stop $containerName
}

# Check if the container exists but is not running
$existingContainer = docker ps -a -q -f "name=$containerName"

if ($existingContainer) {
    Write-Host "Removing stopped container: $containerName"
    docker rm $containerName
}

Write-Host "Building the Docker image..."
docker build -t $containerName .

# Run the container with the correct path to the .env file
Write-Host "Starting a new container: $containerName"
docker run --name $containerName --env-file .env -p 7080:8080 $containerName 

Read-Host -Prompt "Press Enter to exit"
