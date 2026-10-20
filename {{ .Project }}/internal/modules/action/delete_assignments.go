package action

// Delete definitions and unlink their exact codes in one statement. This also
// stays atomic for internal callers without an administration policy. Surviving
// codes retain their stored order; substrings and unrelated grants are untouched.
const deleteAssignedActionsSQL = `
WITH deleted AS (
    DELETE FROM t_action WHERE id = ANY($1) RETURNING code
), users_unlinked AS (
    UPDATE t_user AS owner
    SET action = COALESCE((
        SELECT string_agg(assigned.code, ',' ORDER BY assigned.position)
        FROM unnest(string_to_array(owner.action, ',')) WITH ORDINALITY AS assigned(code, position)
        WHERE NOT EXISTS (SELECT 1 FROM deleted WHERE deleted.code = BTRIM(assigned.code))
    ), ''), updated_at = $2
    WHERE EXISTS (
        SELECT 1 FROM unnest(string_to_array(owner.action, ',')) AS assigned(code)
        JOIN deleted ON deleted.code = BTRIM(assigned.code)
    )
), roles_unlinked AS (
    UPDATE t_role AS owner
    SET action = COALESCE((
        SELECT string_agg(assigned.code, ',' ORDER BY assigned.position)
        FROM unnest(string_to_array(owner.action, ',')) WITH ORDINALITY AS assigned(code, position)
        WHERE NOT EXISTS (SELECT 1 FROM deleted WHERE deleted.code = BTRIM(assigned.code))
    ), ''), updated_at = $2
    WHERE EXISTS (
        SELECT 1 FROM unnest(string_to_array(owner.action, ',')) AS assigned(code)
        JOIN deleted ON deleted.code = BTRIM(assigned.code)
    )
), groups_unlinked AS (
    UPDATE t_user_group AS owner
    SET action = COALESCE((
        SELECT string_agg(assigned.code, ',' ORDER BY assigned.position)
        FROM unnest(string_to_array(owner.action, ',')) WITH ORDINALITY AS assigned(code, position)
        WHERE NOT EXISTS (SELECT 1 FROM deleted WHERE deleted.code = BTRIM(assigned.code))
    ), ''), updated_at = $2
    WHERE EXISTS (
        SELECT 1 FROM unnest(string_to_array(owner.action, ',')) AS assigned(code)
        JOIN deleted ON deleted.code = BTRIM(assigned.code)
    )
)
SELECT COUNT(*) FROM deleted`
