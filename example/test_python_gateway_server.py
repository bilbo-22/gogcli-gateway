import base64
import json
import pathlib
import sys
import unittest

sys.path.insert(0, str(pathlib.Path(__file__).resolve().parent))

import python_gateway_server as gateway


class WebhookResponseContractTest(unittest.TestCase):
    def test_build_webhook_response_encodes_json_body(self):
        resp = gateway.build_webhook_response(
            status_code=418,
            body_obj={"hello": "world"},
            headers={"Content-Type": "application/json"},
        )

        self.assertEqual(resp["status_code"], 418)
        self.assertEqual(resp["headers"]["Content-Type"], "application/json")

        decoded = json.loads(base64.b64decode(resp["body"]).decode("utf-8"))
        self.assertEqual(decoded, {"hello": "world"})

    def test_pending_response_contract(self):
        resp = gateway.pending_webhook_response()

        self.assertEqual(resp["status_code"], 202)
        self.assertEqual(resp["headers"]["Content-Type"], "application/json")

        decoded = json.loads(base64.b64decode(resp["body"]).decode("utf-8"))
        self.assertEqual(decoded["status"], "pending_approval")
        self.assertIn("approval", decoded["message"])


if __name__ == "__main__":
    unittest.main()
