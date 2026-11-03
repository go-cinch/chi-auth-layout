-- Update only the previous built-in wording; preserve edited translations and migration history.
BEGIN;

UPDATE t_dictionary
SET value = jsonb_set(value, '{messages,en-US,backend,AUDIT_FILTER_INVALID}', to_jsonb('Invalid audit filter or time range. Check the configured maximum window.'::text)),
    updated_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
WHERE dictionary_key = 'I18N_CATALOG'
    AND value #>> '{messages,en-US,backend,AUDIT_FILTER_INVALID}' = 'Invalid audit filter or time range (maximum 90 days).';

UPDATE t_dictionary
SET value = jsonb_set(value, '{messages,en-US,backend,AUDIT_EXPORT_LIMIT}', to_jsonb('The export exceeds the configured row limit. Narrow the filters before exporting.'::text)),
    updated_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
WHERE dictionary_key = 'I18N_CATALOG'
    AND value #>> '{messages,en-US,backend,AUDIT_EXPORT_LIMIT}' = 'More than 10,000 events. Narrow the filters before exporting.';

UPDATE t_dictionary
SET value = jsonb_set(value, '{messages,zh-CN,backend,AUDIT_FILTER_INVALID}', to_jsonb('审计筛选条件或时间范围无效，请检查配置的最大时间范围。'::text)),
    updated_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
WHERE dictionary_key = 'I18N_CATALOG'
    AND value #>> '{messages,zh-CN,backend,AUDIT_FILTER_INVALID}' = '审计筛选条件或时间范围无效（最长 90 天）。';

UPDATE t_dictionary
SET value = jsonb_set(value, '{messages,zh-CN,backend,AUDIT_EXPORT_LIMIT}', to_jsonb('超过配置的导出记录上限，请缩小筛选范围后导出。'::text)),
    updated_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
WHERE dictionary_key = 'I18N_CATALOG'
    AND value #>> '{messages,zh-CN,backend,AUDIT_EXPORT_LIMIT}' = '超过 10,000 条记录，请缩小筛选范围后导出。';

COMMIT;
