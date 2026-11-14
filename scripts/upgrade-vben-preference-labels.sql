-- Update untouched stock labels without replacing custom labels or preference values.
UPDATE t_dictionary
SET name = CASE WHEN name = 'Vben PC 默认偏好' THEN 'Vben PC default preferences' ELSE name END,
    description = CASE WHEN description = 'Vben PC 默认界面偏好。用户只保存覆盖项；空时区使用系统时区，空版权日期使用当前年份。语言独立保存在 metadata.language。' THEN 'Default UI preferences for Vben PC. Users store only overrides. An empty timezone follows the system timezone; an empty copyright date uses the current year. Language is stored separately in metadata.language.' ELSE description END,
    updated_at = FLOOR(EXTRACT(EPOCH FROM statement_timestamp()) * 1000)::bigint
WHERE dictionary_key = 'UI_PREFERENCES_VBEN_PC'
  AND (name = 'Vben PC 默认偏好' OR description = 'Vben PC 默认界面偏好。用户只保存覆盖项；空时区使用系统时区，空版权日期使用当前年份。语言独立保存在 metadata.language。');
