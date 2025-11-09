-- init-databases.sql
-- This script runs ONCE when PostgreSQL container first starts
-- It only creates the databases - services will create their own tables

-- Create the three databases your services need
CREATE DATABASE user_service_db;
CREATE DATABASE template_service_db;
CREATE DATABASE notification_db;

-- Enable UUID extension in each database (recommended)
\c user_service_db;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";  -- For text search

\c template_service_db;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

\c notification_db;
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- That's it! No tables needed - services will create them via migrations