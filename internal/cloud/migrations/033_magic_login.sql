ALTER TABLE cloud_tokens DROP CONSTRAINT IF EXISTS cloud_tokens_purpose_check;
ALTER TABLE cloud_tokens ADD CONSTRAINT cloud_tokens_purpose_check CHECK(purpose IN ('verify_email','reset_password','magic_login'));
