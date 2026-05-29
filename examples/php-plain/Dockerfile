FROM docker.io/library/php:8.4-cli-alpine
WORKDIR /app
COPY public ./public
EXPOSE 8080
CMD ["php", "-S", "0.0.0.0:8080", "-t", "public", "public/index.php"]
