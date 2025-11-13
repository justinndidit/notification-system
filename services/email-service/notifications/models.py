from django.db import models

# Create your models here.
class EmailLog(models.Model):
    request_id = models.CharField(max_length=255, unique=True)
    user_id = models.CharField(max_length=255)
    to_email = models.EmailField()
    template_code = models.CharField(max_length=100)
    variables = models.JSONField(default=dict)
    status = models.CharField(max_length=50, default='pending')
    error = models.TextField(null=True, blank=True)
    created_at = models.DateTimeField(auto_now_add=True)
    updated_at = models.DateTimeField(auto_now=True)
    attempts = models.IntegerField(default=0)

    def __str__(self):
        return f"{self.request_id} -> {self.status}"