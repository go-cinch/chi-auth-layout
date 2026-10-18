-- +migrate Up
-- Final scoped permission definitions. Stable codes identify assignments;
-- action_group owns business grouping and explicit bindings own authorization.
-- UI and transport projections match the finalized I18N_CATALOG in 04.
-- Catalog mutations require the explicit developer publish capability or scoped all.
INSERT INTO t_action (id, action_group, code, word, config) VALUES
    (1, 'admin:system:general', 'M6TG8SR9', 'admin:all:system:general', '{
    "ui": [],
    "builtin": "all",
    "endpoints": []
}'::jsonb),
    (2, 'admin:system:general', 'B7RP4HYW', 'admin:default:system:general', '{
    "builtin": "default",
    "endpoints": [
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/auth/change/pwd"
        },
        {
            "protocol": "http",
            "method": "POST",
            "path": "/auth/captcha"
        },
        {
            "protocol": "http",
            "method": "POST",
            "path": "/auth/captcha/verify"
        },
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/msg/inbox/{id}"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/inbox"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/inbox/{id}"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/unread-count"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/msg/inbox/{id}"
        },
        {
            "protocol": "http",
            "method": "POST",
            "path": "/msg/inbox/read-all"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/auth/info"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/auth/reset/pwd"
        },
        {
            "protocol": "http",
            "method": "POST",
            "path": "/auth/challenge"
        }
    ],
    "feature_keys": [
        "admin:account:profile:default-access",
        "admin:msg:inbox:default-access",
        "admin:default:session:access"
    ],
    "resource_keys": [
        "admin:dashboard:overview",
        "admin:account:profile",
        "admin:msg:inbox",
        "admin:default:session"
    ],
    "ui": [
        {
            "menu_key": "admin:menu:dashboard:overview",
            "parent_menu_key": "admin:menu:dashboard",
            "menu_path": "/dashboard/overview",
            "page_entry": true,
            "button_keys": []
        },
        {
            "menu_key": "admin:menu:account:profile",
            "menu_path": "/profile",
            "page_entry": true,
            "button_keys": [
                "admin:btn:account:profile:change-password"
            ]
        },
        {
            "menu_key": "admin:menu:msg:inbox",
            "menu_path": "/msg/inbox",
            "page_entry": true,
            "button_keys": [
                "admin:btn:msg:inbox:query",
                "admin:btn:msg:inbox:refresh",
                "admin:btn:msg:inbox:view-detail",
                "admin:btn:msg:inbox:mark-read",
                "admin:btn:msg:inbox:mark-selected-read",
                "admin:btn:msg:inbox:read-all",
                "admin:btn:msg:inbox:delete",
                "admin:btn:msg:inbox:delete-selected"
            ]
        }
    ]
}'::jsonb),
    (3, 'admin:dashboard:overview', '35VY9TRL', 'admin:read:dashboard:overview', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [],
            "resource_key": "admin:dashboard:overview"
        }
    ],
    "endpoints": [],
    "ui": [
        {
            "menu_key": "admin:menu:dashboard:overview",
            "parent_menu_key": "admin:menu:dashboard",
            "menu_path": "/dashboard/overview",
            "page_entry": true,
            "button_keys": []
        }
    ]
}'::jsonb),
    (4, 'admin:system:user', 'Y6RW5544', 'admin:read:system:user', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user:query",
                "admin:system:user:reset-query",
                "admin:system:user:refresh",
                "admin:system:user:view-detail"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user:query",
                "admin:btn:system:user:reset-query",
                "admin:btn:system:user:refresh",
                "admin:btn:system:user:view-detail"
            ]
        }
    ]
}'::jsonb),
    (5, 'admin:system:user', 'PMWSQYY5', 'admin:create:system:user', '{
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user:create"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "POST",
            "path": "/user"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user:create"
            ]
        }
    ]
}'::jsonb),
    (6, 'admin:system:user', 'R2LTBEEG', 'admin:update:system:user', '{
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user:edit",
                "admin:system:user:approve",
                "admin:system:user:lock",
                "admin:system:user:unlock",
                "admin:system:user:unlock-password"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/user/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user:edit",
                "admin:btn:system:user:approve",
                "admin:btn:system:user:lock",
                "admin:btn:system:user:unlock",
                "admin:btn:system:user:unlock-password"
            ]
        }
    ]
}'::jsonb),
    (7, 'admin:system:user', 'JBVF9YWV', 'admin:delete:system:user', '{
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user:delete",
                "admin:system:user:delete-selected"
            ],
            "resource_key": "admin:system:user"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/user/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user:delete",
                "admin:btn:system:user:delete-selected"
            ]
        }
    ]
}'::jsonb),
    (8, 'admin:system:user-group', 'F9PEAFVW', 'admin:read:system:user-group', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:user-group:query",
                "admin:system:user-group:reset-query",
                "admin:system:user-group:refresh",
                "admin:system:user-group:view-detail",
                "admin:system:user-group:view-members"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user-group"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user-group/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user-group/{id}"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/user-group/{id}/member"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user-group",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:user-group:query",
                "admin:btn:system:user-group:reset-query",
                "admin:btn:system:user-group:refresh",
                "admin:btn:system:user-group:view-detail",
                "admin:btn:system:user-group:view-members"
            ]
        }
    ]
}'::jsonb),
    (9, 'admin:system:user-group', 'VLN9HX2E', 'admin:create:system:user-group', '{
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:user-group:create"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "POST",
            "path": "/user-group"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user-group",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user-group:create"
            ]
        }
    ]
}'::jsonb),
    (10, 'admin:system:user-group', 'K4AQMSC6', 'admin:update:system:user-group', '{
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:user-group:edit",
                "admin:system:user-group:add-members",
                "admin:system:user-group:remove-members"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/user-group/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user-group",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user-group:edit",
                "admin:btn:system:user-group:add-members",
                "admin:btn:system:user-group:remove-members"
            ]
        }
    ]
}'::jsonb),
    (11, 'admin:system:user-group', 'B2VYAELW', 'admin:delete:system:user-group', '{
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:user-group:delete",
                "admin:system:user-group:delete-selected"
            ],
            "resource_key": "admin:system:user-group"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/user-group/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:user-group",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/user-group",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:user-group:delete",
                "admin:btn:system:user-group:delete-selected"
            ]
        }
    ]
}'::jsonb),
    (12, 'admin:system:role', 'XXY567KR', 'admin:read:system:role', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:role:query",
                "admin:system:role:reset-query",
                "admin:system:role:refresh",
                "admin:system:role:view-detail"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/role"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/role/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/role/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/role",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:role:query",
                "admin:btn:system:role:reset-query",
                "admin:btn:system:role:refresh",
                "admin:btn:system:role:view-detail"
            ]
        }
    ]
}'::jsonb),
    (13, 'admin:system:role', '3V5GWBQT', 'admin:create:system:role', '{
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:role:create"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "POST",
            "path": "/role"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/role",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:role:create"
            ]
        }
    ]
}'::jsonb),
    (14, 'admin:system:role', 'QYALLJEG', 'admin:update:system:role', '{
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:role:edit"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/role/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/role",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:role:edit"
            ]
        }
    ]
}'::jsonb),
    (15, 'admin:system:role', 'K5VF772F', 'admin:delete:system:role', '{
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:role:delete",
                "admin:system:role:delete-selected"
            ],
            "resource_key": "admin:system:role"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/role/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:role",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/role",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:role:delete",
                "admin:btn:system:role:delete-selected"
            ]
        }
    ]
}'::jsonb),
    (16, 'admin:system:action', 'E66VGBKA', 'admin:read:system:action', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:action:query",
                "admin:system:action:reset-query",
                "admin:system:action:refresh",
                "admin:system:action:view-detail"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/action"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/action/catalog"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/action/group"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/action/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/action/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/action",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:action:query",
                "admin:btn:system:action:reset-query",
                "admin:btn:system:action:refresh",
                "admin:btn:system:action:view-detail"
            ]
        }
    ]
}'::jsonb),
    (17, 'admin:system:action', '5RNWYB4S', 'admin:create:system:action', '{
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:action:create",
                "admin:system:action:create-group"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/dictionary/{id}"
        },
        {
            "protocol": "http",
            "method": "POST",
            "path": "/action"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/action",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:action:create",
                "admin:btn:system:action:create-group"
            ]
        }
    ]
}'::jsonb),
    (18, 'admin:system:action', 'TW535HD8', 'admin:update:system:action', '{
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:action:edit",
                "admin:system:action:reorder-groups"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/action/{id}"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/dictionary/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/action",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:action:edit",
                "admin:btn:system:action:reorder-groups"
            ]
        }
    ]
}'::jsonb),
    (19, 'admin:system:action', 'V75R23SD', 'admin:delete:system:action', '{
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:action:delete",
                "admin:system:action:delete-selected",
                "admin:system:action:delete-group"
            ],
            "resource_key": "admin:system:action"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/action/{id}"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/dictionary/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:action",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/action",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:action:delete",
                "admin:btn:system:action:delete-selected",
                "admin:btn:system:action:delete-group"
            ]
        }
    ]
}'::jsonb),
    (24, 'admin:system:dictionary', 'H3HW2LEB', 'admin:read:system:dictionary', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:dictionary:query",
                "admin:system:dictionary:reset-query",
                "admin:system:dictionary:refresh",
                "admin:system:dictionary:view-detail"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/dictionary",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:dictionary:query",
                "admin:btn:system:dictionary:reset-query",
                "admin:btn:system:dictionary:refresh",
                "admin:btn:system:dictionary:view-detail"
            ]
        }
    ]
}'::jsonb),
    (25, 'admin:system:dictionary', 'HED6QQTL', 'admin:create:system:dictionary', '{
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:dictionary:create"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "POST",
            "path": "/dictionary"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/dictionary",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:dictionary:create"
            ]
        }
    ]
}'::jsonb),
    (26, 'admin:system:dictionary', 'E3WB6LE7', 'admin:update:system:dictionary', '{
    "bindings": [
        {
            "operation": "update",
            "feature_keys": [
                "admin:system:dictionary:edit"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/dictionary/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/dictionary",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:dictionary:edit"
            ]
        }
    ]
}'::jsonb),
    (27, 'admin:system:dictionary', 'MCHJ3RY3', 'admin:delete:system:dictionary', '{
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:dictionary:delete",
                "admin:system:dictionary:delete-selected"
            ],
            "resource_key": "admin:system:dictionary"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/dictionary/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:dictionary",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/dictionary",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:dictionary:delete",
                "admin:btn:system:dictionary:delete-selected"
            ]
        }
    ]
}'::jsonb),
    (28, 'admin:system:msg', 'JJNWC246', 'admin:read:system:msg', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:system:msg:query",
                "admin:system:msg:reset-query",
                "admin:system:msg:refresh",
                "admin:system:msg:view-detail"
            ],
            "resource_key": "admin:system:msg"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/sent"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/sent/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/sent/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:msg",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/msg",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:msg:query",
                "admin:btn:system:msg:reset-query",
                "admin:btn:system:msg:refresh",
                "admin:btn:system:msg:view-detail"
            ]
        }
    ]
}'::jsonb),
    (29, 'admin:system:msg', '9TS8JETG', 'admin:create:system:msg', '{
    "bindings": [
        {
            "operation": "create",
            "feature_keys": [
                "admin:system:msg:send"
            ],
            "resource_key": "admin:system:msg"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/msg/recipient-option"
        },
        {
            "protocol": "http",
            "method": "POST",
            "path": "/msg"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:msg",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/msg",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:msg:send"
            ]
        }
    ]
}'::jsonb),
    (30, 'admin:system:msg', 'SNP225K5', 'admin:delete:system:msg', '{
    "bindings": [
        {
            "operation": "delete",
            "feature_keys": [
                "admin:system:msg:delete",
                "admin:system:msg:delete-selected"
            ],
            "resource_key": "admin:system:msg"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "DELETE",
            "path": "/msg/sent/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:msg",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/msg",
            "page_entry": false,
            "button_keys": [
                "admin:btn:system:msg:delete",
                "admin:btn:system:msg:delete-selected"
            ]
        }
    ]
}'::jsonb),
    (31, 'admin:audit:login', '47WDGP7H', 'admin:read:audit:login', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:audit:login:query",
                "admin:audit:login:reset-query",
                "admin:audit:login:refresh",
                "admin:audit:login:view-detail",
                "admin:audit:login:export-report"
            ],
            "resource_key": "admin:audit:login"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/login"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/login/detail/{id}"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/login/export"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/login/option"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/login/summary"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:audit:login",
            "parent_menu_key": "admin:menu:audit",
            "menu_path": "/audit/login",
            "page_entry": true,
            "button_keys": [
                "admin:btn:audit:login:query",
                "admin:btn:audit:login:reset-query",
                "admin:btn:audit:login:refresh",
                "admin:btn:audit:login:view-detail",
                "admin:btn:audit:login:export-report"
            ]
        }
    ]
}'::jsonb),
    (33, 'admin:audit:operation', 'GW3JY8SJ', 'admin:read:audit:operation', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [
                "admin:audit:operation:query",
                "admin:audit:operation:reset-query",
                "admin:audit:operation:refresh",
                "admin:audit:operation:view-detail",
                "admin:audit:operation:export-report"
            ],
            "resource_key": "admin:audit:operation"
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/operation"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/operation/detail/{id}"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/operation/export"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/operation/option"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/audit/operation/summary"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:audit:operation",
            "parent_menu_key": "admin:menu:audit",
            "menu_path": "/audit/operation",
            "page_entry": true,
            "button_keys": [
                "admin:btn:audit:operation:query",
                "admin:btn:audit:operation:reset-query",
                "admin:btn:audit:operation:refresh",
                "admin:btn:audit:operation:view-detail",
                "admin:btn:audit:operation:export-report"
            ]
        }
    ]
}'::jsonb),
    (35, 'admin:dashboard:workspace', 'SKFRSH2E', 'admin:read:dashboard:workspace', '{
    "bindings": [
        {
            "operation": "read",
            "feature_keys": [],
            "resource_key": "admin:dashboard:workspace"
        }
    ],
    "endpoints": [],
    "ui": [
        {
            "menu_key": "admin:menu:dashboard:workspace",
            "parent_menu_key": "admin:menu:dashboard",
            "menu_path": "/dashboard/workspace",
            "page_entry": true,
            "button_keys": []
        }
    ]
}'::jsonb),
    (36, 'admin:system:permission-admin', 'HFSNA574', 'admin:grant:system:permission-admin', '{
    "endpoints": [],
    "ui": []
}'::jsonb),
    (37, 'admin:system:developer', '76JJGCBN', 'admin:update:system:developer', '{
    "bindings": [
        {
            "resource_key": "admin:system:developer",
            "operation": "read",
            "feature_keys": [
                "admin:system:developer:compare-online",
                "admin:system:developer:refresh"
            ]
        },
        {
            "resource_key": "admin:system:developer",
            "operation": "update",
            "feature_keys": [
                "admin:system:developer:publish"
            ]
        }
    ],
    "endpoints": [
        {
            "protocol": "http",
            "method": "GET",
            "path": "/action/summary"
        },
        {
            "protocol": "http",
            "method": "GET",
            "path": "/dictionary"
        },
        {
            "protocol": "http",
            "method": "PATCH",
            "path": "/dictionary/{id}"
        }
    ],
    "ui": [
        {
            "menu_key": "admin:menu:system:developer",
            "parent_menu_key": "admin:menu:system",
            "menu_path": "/system/developer",
            "page_entry": true,
            "button_keys": [
                "admin:btn:system:developer:compare-online",
                "admin:btn:system:developer:refresh",
                "admin:btn:system:developer:publish"
            ]
        }
    ]
}'::jsonb);

SELECT setval(pg_get_serial_sequence('t_action', 'id'), (SELECT MAX(id) FROM t_action), TRUE);

-- +migrate Down
TRUNCATE TABLE t_action RESTART IDENTITY;
