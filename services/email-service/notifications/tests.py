"""
Tests for the queue bridge's payload translation.

The orchestrator and the Celery task are in different languages with no shared
type, so the only thing keeping them in step is this translation and the
contract in docs/contracts/enriched-notification.md. Every integration bug this
service has had lived exactly here.
"""

from django.test import SimpleTestCase

from notifications.management.commands.consume_notifications import to_task_payload


def enriched_message(**overrides):
    """A contract-shaped message, as the orchestrator publishes it."""
    message = {
        "notification_id": "0f7c2c1e-6d1a-4a6f-9a3a-2b1f4c0d9e88",
        "correlation_id": "b2a1f0c9-1234-4d5e-8f90-1a2b3c4d5e6f",
        "idempotency_key": "req-123",
        "user_id": "9c8b7a65-4321-4f0e-9d8c-7b6a5f4e3d2c",
        "template_code": "3e2d1c0b-9a87-4655-b4a3-2c1d0e9f8a7b",
        "channel": "email",
        "priority": "normal",
        "recipient": "ada@example.com",
        "subject": "Welcome, Ada",
        "body": "<p>Hello Ada</p>",
        "variables": {"name": "Ada", "link": "https://example.com"},
        "metadata": {},
    }
    message.update(overrides)
    return message


class ToTaskPayloadTests(SimpleTestCase):
    def test_carries_the_notification_id_separately_from_request_id(self):
        """
        The status callback keys on the notification UUID. request_id is the
        caller's idempotency key and is not interchangeable with it — reporting
        the wrong one made every callback fail validation.
        """
        payload = to_task_payload(enriched_message())

        self.assertEqual(
            payload["notification_id"], "0f7c2c1e-6d1a-4a6f-9a3a-2b1f4c0d9e88"
        )
        self.assertEqual(payload["request_id"], "req-123")
        self.assertNotEqual(payload["notification_id"], payload["request_id"])

    def test_falls_back_to_the_notification_id_when_no_idempotency_key(self):
        payload = to_task_payload(enriched_message(idempotency_key=None))

        self.assertEqual(
            payload["request_id"], "0f7c2c1e-6d1a-4a6f-9a3a-2b1f4c0d9e88"
        )

    def test_resolved_recipient_reaches_the_task(self):
        """
        The task reads the address out of `variables`, while the contract carries
        it as a resolved top-level field.
        """
        payload = to_task_payload(enriched_message())

        self.assertEqual(payload["variables"]["email"], "ada@example.com")

    def test_rendered_content_is_passed_through(self):
        """
        Content is rendered by the orchestrator. If it did not survive
        translation the task would silently fall back to fetching a template.
        """
        payload = to_task_payload(enriched_message())

        self.assertEqual(payload["subject"], "Welcome, Ada")
        self.assertEqual(payload["body"], "<p>Hello Ada</p>")
        self.assertEqual(payload["variables"]["subject"], "Welcome, Ada")

    def test_correlation_id_is_preserved_for_tracing(self):
        payload = to_task_payload(enriched_message())

        self.assertEqual(
            payload["metadata"]["correlation_id"],
            "b2a1f0c9-1234-4d5e-8f90-1a2b3c4d5e6f",
        )
        self.assertEqual(
            payload["metadata"]["notification_id"],
            "0f7c2c1e-6d1a-4a6f-9a3a-2b1f4c0d9e88",
        )

    def test_original_variables_are_not_discarded(self):
        payload = to_task_payload(enriched_message())

        self.assertEqual(payload["variables"]["name"], "Ada")
        self.assertEqual(payload["variables"]["link"], "https://example.com")

    def test_missing_optional_fields_do_not_raise(self):
        """A message without metadata or variables must still translate."""
        message = enriched_message()
        del message["metadata"]
        del message["variables"]

        payload = to_task_payload(message)

        self.assertEqual(payload["variables"]["email"], "ada@example.com")
        self.assertIn("correlation_id", payload["metadata"])

    def test_does_not_mutate_the_incoming_message(self):
        """
        The consumer nacks and requeues on failure, so the message it holds must
        not have been altered by a partial translation.
        """
        message = enriched_message()
        original_variables = dict(message["variables"])

        to_task_payload(message)

        self.assertEqual(message["variables"], original_variables)

    def test_channel_defaults_to_email(self):
        message = enriched_message()
        del message["channel"]

        self.assertEqual(to_task_payload(message)["notification_type"], "email")
