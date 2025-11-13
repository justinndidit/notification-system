# Use your Python base image
FROM python:3.11-slim

WORKDIR /app

# Copy requirements and install
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copy the project
COPY . .

# Expose port if needed (optional)
EXPOSE 8000

# Command to run Celery
CMD ["celery", "-A", "email_service", "worker", "--loglevel=info"]
