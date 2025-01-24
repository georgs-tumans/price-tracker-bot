#!/bin/bash

container_name="price_tracker_bot"

# Move to the script's directory
cd "$(dirname "$0")"

# Go to the project root (assuming 'deployment' is a subfolder)
cd ..

# Check if the container is running
running_container=$(docker ps -q -f "name=$container_name")

if [ -n "$running_container" ]; then
    echo "Stopping running container: $container_name"
    docker stop $container_name
fi

# Check if the container exists but is not running
existing_container=$(docker ps -a -q -f "name=$container_name")

if [ -n "$existing_container" ]; then
    echo "Removing stopped container: $container_name"
    docker rm $container_name
fi

echo "Building the Docker image..."
docker build -t $container_name .  # The dot refers to the project root

# Run the container
echo "Starting a new container: $container_name"
docker run --name $container_name --env-file .env -p 7080:8080 -d $container_name

echo "Deployment completed."
