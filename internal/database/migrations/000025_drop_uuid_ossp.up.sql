-- PostgreSQL 18: Drop uuid-ossp extension
-- PG 18 has native uuidv4() and uuidv7() functions, uuid-ossp no longer needed
-- Note: pgcrypto is still required for password hashing (crypt/gen_salt)

DROP EXTENSION IF EXISTS "uuid-ossp";

COMMENT ON EXTENSION pgcrypto IS 'Password hashing with crypt() and gen_salt()';
