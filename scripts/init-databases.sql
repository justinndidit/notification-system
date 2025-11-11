-- init-databases.sql
-- This script runs ONCE when PostgreSQL container first starts.
-- It creates the three necessary databases and enables shared extensions/functions in each.

-- =====================================================
-- 1. CREATE DATABASES
-- =====================================================
CREATE DATABASE user_service_db;
CREATE DATABASE template_service_db;
CREATE DATABASE notification_db;

-- =====================================================
-- 2. APPLY EXTENSIONS AND FUNCTIONS TO EACH DB
-- =====================================================

-- Connect to user_service_db
\c user_service_db;

-- Enable UUID and text search extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Create the shared update_at function in this DB
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- Connect to template_service_db
\c template_service_db;

-- Enable UUID and text search extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Create the shared update_at function in this DB
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- Connect to notification_db
\c notification_db;

-- Enable UUID and text search extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pg_trgm";

-- Create the shared update_at function in this DB
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Completed setup! Services will now run their migrations to create tables.