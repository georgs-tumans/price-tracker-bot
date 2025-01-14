# Step 1: Use the official Go image to build the app
FROM golang:1.23.2-alpine AS build

# Step 2: Set the working directory inside the container
WORKDIR /app

# Step 3: Copy the Go app source code into the container
COPY . .

# Step 4: Download Go modules and build the app
RUN go mod download
RUN go build -o price_tracker_bot .

# Step 5: Use a smaller image for the final build
FROM alpine:latest

# Install timezone data
RUN apk add --no-cache tzdata

# Step 6: Set the working directory in the new smaller image
WORKDIR /root/

# Step 7: Copy the built Go binary from the build stage
COPY --from=build /app/price_tracker_bot .
COPY ./tracker_configs tracker_configs

# Step 8: Set timezone from environment variable (default to UTC if not provided)
ENV TZ=${TZ:-UTC}

# Step 9: Run the Go app
CMD ["./price_tracker_bot"]
