FROM golang:1.23-bullseye

# Add ARM64 architecture
RUN dpkg --add-architecture arm64

# Update package lists
RUN apt-get update

# Install cross-compilation tools for ARM64
RUN apt-get install -y \
    gcc-aarch64-linux-gnu \
    libc6-dev-arm64-cross \
    pkg-config

# Install ARM64 versions of libraries
RUN apt-get install -y \
    portaudio19-dev:arm64 \
    libasound2-dev:arm64 \
    && rm -rf /var/lib/apt/lists/*

# Set environment for ARM64 cross-compilation
ENV CC=aarch64-linux-gnu-gcc
ENV CXX=aarch64-linux-gnu-g++
ENV AR=aarch64-linux-gnu-ar
ENV STRIP=aarch64-linux-gnu-strip

# Configure pkg-config for cross-compilation
ENV PKG_CONFIG_PATH=/usr/lib/aarch64-linux-gnu/pkgconfig
ENV PKG_CONFIG_LIBDIR=/usr/lib/aarch64-linux-gnu/pkgconfig
ENV PKG_CONFIG_SYSROOT_DIR=/
ENV PKG_CONFIG_ALLOW_CROSS=1

WORKDIR /src