-- One database per service: services never share a database.
-- Runs only when the volume is empty. A service added later appends its
-- CREATE DATABASE here and is applied with: docker compose down -v && docker compose up -d
CREATE DATABASE order_service;
CREATE DATABASE wallet_service;
