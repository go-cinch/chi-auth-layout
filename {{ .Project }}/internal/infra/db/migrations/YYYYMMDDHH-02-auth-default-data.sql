-- +migrate Up
-- Required super account and role in both seed modes.
-- Assignment codes are resolved by the following action seed migration before startup completes.
INSERT INTO t_role (id, name, word, action) VALUES
    (1, 'Super', 'super', 'M6TG8SR9');

-- The initial super password is super.
INSERT INTO t_user (id, role_id, username, code, password, status, action) VALUES
    (1, 1, 'super', 'RWBAAHC5', '$2a$10$Wx41MADqB3kuLD/bdD8DIeph55Oeo9HABKGt2p2ilqdkHAfZ7EGPm', 1, 'B7RP4HYW');

{{ if not .Computed.only_super_final -}}
-- Additional example identities are included only when only_super=false.
INSERT INTO t_role (id, name, word, action) VALUES
    (2, 'Guest', 'guest', '35VY9TRL'),
    (3, 'Admin', 'admin', 'B7RP4HYW,35VY9TRL,Y6RW5544,PMWSQYY5,R2LTBEEG,JBVF9YWV,F9PEAFVW,VLN9HX2E,K4AQMSC6,B2VYAELW,XXY567KR,3V5GWBQT,QYALLJEG,K5VF772F,E66VGBKA,5RNWYB4S,TW535HD8,V75R23SD,JJNWC246,9TS8JETG,SNP225K5,47WDGP7H,GW3JY8SJ,SKFRSH2E,HFSNA574'),
    (4, 'Developer', 'developer', 'B7RP4HYW,35VY9TRL,Y6RW5544,PMWSQYY5,R2LTBEEG,JBVF9YWV,F9PEAFVW,VLN9HX2E,K4AQMSC6,B2VYAELW,XXY567KR,3V5GWBQT,QYALLJEG,K5VF772F,E66VGBKA,5RNWYB4S,TW535HD8,V75R23SD,H3HW2LEB,HED6QQTL,E3WB6LE7,MCHJ3RY3,JJNWC246,9TS8JETG,SNP225K5,47WDGP7H,GW3JY8SJ,SKFRSH2E,HFSNA574,76JJGCBN'),
    (5, 'Operator', 'operator', 'B7RP4HYW,SKFRSH2E');

-- Demo groups cover all ordinary admin pages, including workspace, dictionaries,
-- message management and both audit pages. Developer/catalog publishing remains
-- a separate capability of the Developer role, never implied by a demo tier.
-- Read Only: read. Read Write: read/create/update/delete. No Delete: read/create/update.
-- Personal account, inbox and session defaults remain assigned directly below.
INSERT INTO t_user_group (id, name, word, action) VALUES
    (1, 'Read Only', 'readonly', '35VY9TRL,SKFRSH2E,Y6RW5544,F9PEAFVW,XXY567KR,E66VGBKA,H3HW2LEB,JJNWC246,47WDGP7H,GW3JY8SJ'),
    (2, 'Read Write', 'write', '35VY9TRL,SKFRSH2E,Y6RW5544,PMWSQYY5,R2LTBEEG,JBVF9YWV,F9PEAFVW,VLN9HX2E,K4AQMSC6,B2VYAELW,XXY567KR,3V5GWBQT,QYALLJEG,K5VF772F,E66VGBKA,5RNWYB4S,TW535HD8,V75R23SD,H3HW2LEB,HED6QQTL,E3WB6LE7,MCHJ3RY3,JJNWC246,9TS8JETG,SNP225K5,47WDGP7H,GW3JY8SJ'),
    (3, 'No Delete', 'nodelete', '35VY9TRL,SKFRSH2E,Y6RW5544,PMWSQYY5,R2LTBEEG,F9PEAFVW,VLN9HX2E,K4AQMSC6,XXY567KR,3V5GWBQT,QYALLJEG,E66VGBKA,5RNWYB4S,TW535HD8,H3HW2LEB,HED6QQTL,E3WB6LE7,JJNWC246,9TS8JETG,47WDGP7H,GW3JY8SJ');

-- Local seed credentials intentionally use the username as the password.
INSERT INTO t_user (id, role_id, username, code, password, status, action) VALUES
    (2, 2, 'guest', 'VWKNYWNF', '$2a$10$DzWbkb12WHoyj5UPIAdHBOYXFi6jn5gzQRUoOU8c6oZ9XJx9w6Tju', 1, 'B7RP4HYW'),
    (3, NULL, 'readonly', 'SWS8MCD6', '$2a$10$e5bhTbZUo8JLBWOfxWY7A.BpiGCTgvV3PxIhPozM9UolwFuENjIZi', 1, 'B7RP4HYW'),
    (4, NULL, 'write', 'C4FL7VGG', '$2a$10$SB4cqKKE5t9ACvMXtHaJuOafN1nvJgdK.5XxVAITBCSsU.gSNTbAm', 1, 'B7RP4HYW'),
    (5, NULL, 'nodelete', 'RSY34NYR', '$2a$10$.QZNNNrOuEdPvV0UdYwEae942Bb7Hr28Zb0vrbBRocHUkSpqk/Uqu', 1, 'B7RP4HYW'),
    (6, 3, 'admin', 'RYQQFLEF', '$2a$10$30p0/2rXDEQMlENvObbsOOWEcQ/UtK7HwTnSph0OWg9YSXWWWADjm', 1, 'B7RP4HYW'),
    (7, 4, 'developer', 'PC787YK3', '$2a$10$27TEL7BVoUIC1eBQaOsIueLPuoID2VA.2aIXAy4kcI5e3IaxwDIl.', 1, 'B7RP4HYW'),
    (8, 5, 'operator', 'GT8229RP', '$2a$10$ibw1wSobR/4DJhrPPey6.ekHt/1XuPwzf16D.MUpWQQHHz9k/yGJe', 1, 'B7RP4HYW');

INSERT INTO t_user_user_group_relation (user_id, user_group_id) VALUES
    (3, 1),
    (4, 2),
    (5, 3);

{{ end -}}
-- A fresh schema has no messages; initialize zero-count statistics for seed users.
INSERT INTO t_user_msg_status (user_id)
SELECT id
FROM t_user
ORDER BY id;

SELECT setval(pg_get_serial_sequence('t_role', 'id'), (SELECT MAX(id) FROM t_role), TRUE);
{{ if not .Computed.only_super_final -}}
SELECT setval(pg_get_serial_sequence('t_user_group', 'id'), (SELECT MAX(id) FROM t_user_group), TRUE);
{{ end -}}
SELECT setval(pg_get_serial_sequence('t_user', 'id'), (SELECT MAX(id) FROM t_user), TRUE);

-- +migrate Down
-- Undo only the tables seeded above. DELETE avoids requiring every table
-- referencing t_user to participate in a TRUNCATE.
DELETE FROM t_user_msg_status;

{{ if not .Computed.only_super_final -}}
DELETE FROM t_user_user_group_relation;

{{ end -}}
DELETE FROM t_user;

{{ if not .Computed.only_super_final -}}
DELETE FROM t_user_group;

{{ end -}}
DELETE FROM t_role;

SELECT setval(pg_get_serial_sequence('t_user_msg_status', 'id'), 1, FALSE);
{{ if not .Computed.only_super_final -}}
SELECT setval(pg_get_serial_sequence('t_user_user_group_relation', 'id'), 1, FALSE);
SELECT setval(pg_get_serial_sequence('t_user_group', 'id'), 1, FALSE);
{{ end -}}
SELECT setval(pg_get_serial_sequence('t_user', 'id'), 1, FALSE);
SELECT setval(pg_get_serial_sequence('t_role', 'id'), 1, FALSE);
