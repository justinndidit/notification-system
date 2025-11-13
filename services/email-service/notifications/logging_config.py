"""
Logging configuration with correlation ID tracking.
"""

import logging
import json
from datetime import datetime
from typing import Optional
import uuid


class CorrelationIdFilter(logging.Filter):
    """
    Filter to add correlation ID to all log records.
    """

    def __init__(self, name: str = ""):
        super().__init__(name)
        self.correlation_id = None

    def filter(self, record):
        if self.correlation_id:
            record.correlation_id = self.correlation_id
        else:
            record.correlation_id = str(uuid.uuid4())
        return True


class JsonFormatter(logging.Formatter):
    """
    Format logs as JSON for structured logging.
    """

    def format(self, record):
        log_data = {
            "timestamp": datetime.utcnow().isoformat(),
            "level": record.levelname,
            "logger": record.name,
            "message": record.getMessage(),
            "correlation_id": getattr(record, "correlation_id", "unknown"),
            "module": record.module,
            "function": record.funcName,
            "line": record.lineno,
        }

        if record.exc_info:
            log_data["exception"] = self.formatException(record.exc_info)

        return json.dumps(log_data)


def setup_logging(
    name: str,
    log_level: str = "INFO",
    use_json: bool = True,
) -> logging.Logger:
    """
    Configure logger with correlation ID and structured logging.

    Args:
        name: Logger name
        log_level: Logging level
        use_json: Whether to use JSON formatting

    Returns:
        Configured logger instance
    """
    logger = logging.getLogger(name)
    logger.setLevel(getattr(logging, log_level.upper()))

    # Remove existing handlers
    logger.handlers = []

    # Create console handler
    handler = logging.StreamHandler()
    handler.setLevel(getattr(logging, log_level.upper()))

    # Add correlation ID filter
    correlation_filter = CorrelationIdFilter()
    handler.addFilter(correlation_filter)

    # Set formatter
    if use_json:
        formatter = JsonFormatter()
    else:
        formatter = logging.Formatter(
            "%(asctime)s - %(name)s - %(levelname)s - [%(correlation_id)s] - %(message)s"
        )

    handler.setFormatter(formatter)
    logger.addHandler(handler)

    return logger


# Create global logger instances
logger = setup_logging("email_service")
celery_logger = setup_logging("email_service.celery")
utils_logger = setup_logging("email_service.utils")
