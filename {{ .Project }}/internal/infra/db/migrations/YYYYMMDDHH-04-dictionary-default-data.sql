-- +migrate Up
-- Seed captcha dictionaries and the finalized localization/resource catalog.
INSERT INTO t_dictionary (id, dictionary_key, name, value, description) VALUES
    (1, 'POINT_CAPTCHA_ENGLISH_CHARACTERS', 'Point Captcha English Characters', '["A","B","C","D","E","F","G","H","I","J","K","L","M","N","O","P","Q","R","S","T","U","V","W","X","Y","Z","0","1","2","3","4","5","6","7","8","9"]'::jsonb, 'Uppercase English letters and digits available to point captcha challenges.'),
    (2, 'POINT_CAPTCHA_CHINESE_CHARACTERS', 'Point Captcha Chinese Characters', '["啊","挨","皑","艾","鞍","俺","胺","盎","翱","奥","捌","笆","拔","把","罢","百","拜","班","颁","拌","办","帮","绑","镑","胞","剥","堡","报","爆","悲","背","狈","被","本","甭","逼","笔","蓖","毖","闭","辟","避","编","变","辫","彪","憋","斌","摈","柄","炳","菠","波","铂","帛","渤","卜","不","簿","猜","财","彩","餐","惭","舱","操","曹","侧","蹭","茶","搽","诧","搀","谗","产","猖","长","敞","倡","朝","吵","撤","澈","尘","陈","称","成","惩","逞","痴","池","耻","赤","充","崇","畴","筹","丑","出","锄","楚","搐","川","传","疮","闯","捶","春","淳","绰","雌","瓷","赐","囱","凑","簇","窜","脆","翠","寸","措","达","大","傣","代","逮","丹","掸","但","弹","党","刀","岛","稻","德","蹬","瞪","低","笛","翟","地","弟","掂","点","电","惦","碉","凋","钓","碟","叠","钉","锭","东","动","冻","抖","逗","毒","堵","杜","渡","锻","堆","对","敦","盾","多","躲","剁","峨","讹","厄","饿","耳","二","罚","阀","帆","矾","凡","范","泛","肪","妨","放","飞","吠","沸","吩","坟","奋","愤","枫","风","冯","奉","夫","扶","氟","服","福","抚","斧","腐","覆","付","负","妇","噶","概","干","竿","感","冈","肛","杠","高","搞","哥","鸽","割","蛤","个","跟","庚","梗","恭","公","巩","共","苟","购","菇","沽","古","股","固","剐","乖","关","观","惯","广","圭","龟","诡","跪","滚","郭","过","海","骇","韩","寒","翰","憾","汉","航","毫","号","荷","禾","盒","涸","贺","痕","哼","恒","虹","宏","侯","候","忽","葫","糊","唬","户","华","划","徊","欢","还","唤","涣","慌","蝗","惶","恍","辉","蛔","慧","贿","汇","绘","魂","活","或","货","基","积","迹","姬","吉","籍","疾","级","脊","冀","剂","寂","既","继","夹","荚","甲","价","歼","笺","兼","缄","碱","简","减","践","箭","剑","溅","姜","疆","讲","降","焦","浇","搅","侥","饺","教","叫","接","阶","桔","竭","解","芥","疥","筋","今","锦","靳","烬","荆","睛","惊","井","静","镜","竟","窘","玖","灸","救","咎","拘","居","咀","聚","具","锯","炬","娟","绢","抉","觉","均","君","竣","喀","开","慨","勘","康","抗","拷","坷","磕","咳","刻","肯","坑","孔","扣","哭","库","挎","筷","宽","狂","旷","岿","魁","愧","捆","廓","喇","辣","赖","拦","澜","览","滥","廊","捞","老","烙","雷","累","擂","棱","梨","狸","理","礼","栗","砾","傈","立","力","联","镰","帘","恋","凉","良","晾","聊","寥","了","料","劣","磷","邻","赁","菱","伶","灵","另","榴","刘","柳","咙","垄","娄","陋","颅","卤","碌","鹿","录","吕","履","氯","滤","孪","掠","伦","纶","罗","骡","骆","麻","马","埋","迈","馒","曼","芒","忙","茅","铆","帽","玫","酶","眉","美","媚","们","盟","孟","糜","弥","泌","棉","免","缅","瞄","庙","民","敏","螟","命","蘑","摩","末","沫","谋","牡","母","募","睦","哪","那","乃","南","挠","闹","内","霓","拟","腻","拈","捻","鸟","聂","镍","狞","泞","钮","农","怒","疟","糯","鸥","偶","爬","琶","徘","潘","畔","乓","胖","炮","呸","裴","佩","砰","彭","硼","鹏","砒","劈","脾","匹","譬","骗","票","拼","聘","萍","评","泼","魄","扑","葡","埔","浦","期","妻","漆","棋","崎","祈","起","启","气","泣","恰","钎","签","黔","前","谴","歉","羌","强","敲","乔","撬","俏","且","侵","勤","禽","轻","清","情","庆","丘","囚","区","屈","取","去","醛","拳","劝","却","雀","燃","瓤","让","惹","人","任","纫","戎","融","容","柔","儒","乳","褥","瑞","若","萨","塞","伞","丧","嫂","涩","砂","纱","筛","苫","煽","擅","汕","墒","晌","梢","芍","少","奢","舍","慑","设","伸","绅","婶","慎","甥","省","圣","施","尸","拾","蚀","矢","驶","士","拭","是","适","饰","室","手","寿","瘦","梳","叔","疏","熟","署","属","树","墅","恕","衰","栓","爽","税","舜","朔","嘶","丝","嗣","似","耸","宋","艘","苏","速","溯","酸","虽","髓","遂","损","梭","索","他","獭","胎","台","态","贪","檀","谭","袒","炭","堂","唐","淌","涛","桃","陶","藤","梯","提","啼","惕","天","甜","腆","眺","帖","汀","亭","通","同","桶","统","头","突","涂","吐","推","褪","臀","脱","驼","唾","洼","袜","弯","丸","挽","惋","腕","枉","望","巍","韦","唯","维","伟","纬","畏","位","慰","温","纹","问","挝","窝","握","钨","屋","梧","武","舞","戊","物","误","西","嘻","牺","悉","熄","犀","席","铣","戏","匣","暇","下","掀","鲜","贤","涎","险","腺","宪","相","箱","翔","想","巷","象","削","消","晓","肖","楔","鞋","携","谐","蟹","谢","芯","新","衅","惺","型","醒","姓","匈","熊","朽","袖","需","须","酗","畜","绪","喧","玄","绚","穴","勋","询","殉","逊","押","丫","崖","哑","焉","淹","蜒","颜","沿","衍","燕","唁","宴","央","扬","洋","仰","漾","瑶","窑","舀","耀","爷","页","曳","液","揖","衣","移","疑","彝","已","艺","邑","臆","亦","忆","溢","译","绎","殷","姻","寅","隐","婴","缨","荧","盈","映","佣","雍","泳","勇","优","由","油","友","釉","迂","榆","余","鱼","隅","与","语","域","遇","愈","誉","预","鸳","垣","辕","猿","远","院","跃","月","耘","陨","酝","匝","栽","载","攒","脏","糟","早","噪","燥","则","增","扎","轧","眨","乍","斋","寨","詹","斩","蘸","站","樟","张","丈","胀","招","赵","肇","哲","者","浙","甄","针","疹","镇","睁","怔","政","郑","支","肢","织","植","侄","趾","志","至","峙","稚","滞","中","衷","重","周","诌","帚","昼","蛛","诸","烛","嘱","助","铸","祝","拽","撰","桩","撞","锥","缀","捉","琢","着","咨","滋","仔","自","棕","综","走","足","诅","钻","最","昨","做"]'::jsonb, 'One thousand common simplified Chinese characters sampled from the GB2312 level-one repertoire.');

-- Load this catalog after migrations and before localized application services are made ready.
-- Language registration and label lookup use data; runtime loaders must not overwrite edited messages with bundled files.
-- This finalized catalog is the baseline exported from the development dictionary.
-- Directory/resource names are canonical; menu labels are projected at runtime.
-- Button labels use independent control IDs from action.config.ui[].button_keys, not action words.
INSERT INTO t_dictionary (id, dictionary_key, name, value, description, enabled) VALUES (
    3,
    'I18N_CATALOG',
    'Application localization catalog',
    '{
    "scopes": [
        "admin",
        "h5",
        "mini",
        "app"
    ],
    "locales": [
        {
            "tag": "zh-CN",
            "name": "简体中文",
            "enabled": true
        },
        {
            "tag": "en-US",
            "name": "English",
            "enabled": true
        }
    ],
    "messages": {
        "en-US": {
            "backend": {
                "ROLE_IDS": "provide 1-100 positive role ids",
                "USER_IDS": "provide 1-100 positive user ids",
                "ACTION_IDS": "provide 1-100 positive action ids",
                "AUTH_LOCKED": "user is locked",
                "MSG_INVALID": "Invalid message fields: title up to 200 characters, content up to 20000, and expiry must be in the future",
                "AUTH_INVALID": "invalid login",
                "ROLE_INVALID": "invalid role",
                "USER_INVALID": "invalid user",
                "HTTP_INTERNAL": "internal server error",
                "MSG_NOT_FOUND": "Message not found or unavailable",
                "ROLE_CONFLICT": "role word already exists",
                "USERGROUP_IDS": "provide 1-100 positive user group ids",
                "USER_CONFLICT": "username already exists",
                "ACTION_INVALID": "invalid action",
                "DICTIONARY_IDS": "provide 1-100 positive dictionary ids",
                "HTTP_FORBIDDEN": "Forbidden",
                "HTTP_NOT_FOUND": "Not Found",
                "ROLE_NOT_FOUND": "role not found",
                "USER_NOT_FOUND": "user not found",
                "ACTION_CONFLICT": "action code or word already exists",
                "AUDIT_NOT_FOUND": "Audit event not found.",
                "ACTION_NOT_FOUND": "action not found",
                "HTTP_BAD_REQUEST": "Bad Request",
                "AUTH_LOGIN_FAILED": "invalid username or password",
                "AUTH_UNAUTHORIZED": "unauthorized",
                "HTTP_INVALID_BODY": "invalid request body",
                "HTTP_INVALID_PAGE": "p must be one int32 value",
                "HTTP_UNAUTHORIZED": "Unauthorized",
                "USERGROUP_INVALID": "invalid user group",
                "AUDIT_EXPORT_LIMIT": "More than 10,000 events. Narrow the filters before exporting.",
                "AUTH_SAME_PASSWORD": "new password must differ from current password",
                "DICTIONARY_INVALID": "invalid dictionary",
                "HTTP_INVALID_QUERY": "invalid query parameters",
                "USERGROUP_CONFLICT": "user group word already exists",
                "DICTIONARY_CONFLICT": "dictionary key already exists",
                "USERGROUP_NOT_FOUND": "user group not found",
                "USER_ROLE_NOT_FOUND": "user role not found",
                "AUDIT_FILTER_INVALID": "Invalid audit filter or time range (maximum 90 days).",
                "AUTH_USERNAME_EXISTS": "username already exists",
                "DICTIONARY_NOT_FOUND": "dictionary not found",
                "HTTP_CAPTCHA_EXPIRED": "point selection challenge expired",
                "HTTP_GATEWAY_TIMEOUT": "Gateway Timeout",
                "HTTP_INVALID_ROLE_ID": "invalid role id",
                "HTTP_INVALID_USER_ID": "invalid user id",
                "AUTH_PENDING_APPROVAL": "user registration is pending approval",
                "MSG_INVALID_RECIPIENT": "Choose 1–1000 existing users for targeted messages; broadcasts cannot specify recipients",
                "ROLE_ACTION_NOT_FOUND": "role action not found",
                "USER_ACTION_NOT_FOUND": "user action not found",
                "AUTH_SESSION_NOT_FOUND": "authentication session not found",
                "HTTP_DUPLICATE_REQUEST": "idempotency key has already been used",
                "HTTP_INVALID_ACTION_ID": "invalid action id",
                "HTTP_INVALID_PAGE_SIZE": "s must be one int32 value",
                "AUTH_INCORRECT_PASSWORD": "current password is incorrect",
                "AUTH_INFO_SCOPE_INVALID": "Invalid permission scope",
                "AUTH_INVALID_CREDENTIAL": "invalid login credential",
                "HTTP_METHOD_NOT_ALLOWED": "Method Not Allowed",
                "SYSTEM_FEATURE_DISABLED": "this feature is disabled by the system",
                "AUTH_CHALLENGE_NOT_FOUND": "credential challenge not found",
                "AUTH_INFO_SCOPE_REQUIRED": "Please specify a permission scope",
                "HTTP_INVALID_USER_STATUS": "status must be comma-separated values from 0 to 2",
                "HTTP_SERVICE_UNAVAILABLE": "Service Unavailable",
                "USERGROUP_USER_NOT_FOUND": "user group member not found",
                "AUTH_INVALID_REGISTRATION": "invalid registration",
                "HTTP_INVALID_DICTIONARY_ID": "invalid dictionary id",
                "HTTP_INVALID_KEYWORD_QUERY": "keyword must be one value",
                "HTTP_INVALID_USER_GROUP_ID": "invalid user group id",
                "USERGROUP_ACTION_NOT_FOUND": "user group action not found",
                "AUTH_INVALID_PASSWORD_RESET": "New password is required",
                "AUTH_PASSWORD_CHANGE_LOCKED": "self-service password change is locked; please contact an administrator",
                "AUTH_POINT_CAPTCHA_REQUIRED": "invalid captcha",
                "DICTIONARY_VERSION_CONFLICT": "The catalog changed. Compare the latest online version before submitting.",
                "HTTP_INVALID_CATEGORY_VALUE": "category must be 0 or 1",
                "HTTP_INVALID_PUBLIC_PURPOSE": "purpose must be login or register",
                "HTTP_INVALID_USERNAME_QUERY": "username must be one value",
                "AUTH_INVALID_PASSWORD_CHANGE": "invalid password change",
                "AUTH_PASSWORD_RESET_REQUIRED": "Reset your password before continuing",
                "AUTH_POINT_CAPTCHA_NOT_FOUND": "point captcha not found",
                "AUTH_SLIDER_CAPTCHA_REQUIRED": "slider verification is required",
                "HTTP_INVALID_CATEGORY_FORMAT": "category must be one int16 value",
                "HTTP_INVALID_IDEMPOTENCY_KEY": "invalid idempotency key",
                "HTTP_INVALID_PASSWORD_PURPOSE": "purpose must be password_change or password_reset",
                "AUTH_PERMISSION_ADMIN_REQUIRED": "Only an administrator with all permissions in the current scope can change permission definitions or account credentials",
                "AUTH_PERMISSION_GRANT_REQUIRED": "All permissions or the permission management grant in the current scope is required",
                "HTTP_INVALID_DICTIONARY_ENABLED": "enabled must be one boolean value",
                "AUTH_INVALID_PASSWORD_CREDENTIAL": "invalid password change credential",
                "AUTH_PASSWORD_RESET_NOT_REQUIRED": "Password reset is not required for this account",
                "AUTH_INVALID_REGISTRATION_CREDENTIAL": "invalid registration credential"
            },
            "frontend": {
                "ui": {
                    "crop": {
                        "title": "Image Cropping",
                        "cancel": "Cancel cropping",
                        "confirm": "Crop",
                        "errorTip": "Cropping error",
                        "titleTip": "Cropping Ratio {0}"
                    },
                    "tiptap": {
                        "upload": {
                            "fileTooLarge": "File size exceeds the limit",
                            "uploadFailed": "Upload Failed",
                            "onlySingleImage": "Only single image upload is supported, the first one is selected",
                            "fileTypeNotAllowed": "File type is not allowed"
                        },
                        "prompts": {
                            "link": "Enter link URL",
                            "image": "Enter image URL"
                        },
                        "toolbar": {
                            "bold": "Bold",
                            "code": "Code",
                            "link": "Link",
                            "redo": "Redo",
                            "undo": "Undo",
                            "clear": "Clear",
                            "image": "Image",
                            "italic": "Italic",
                            "strike": "Strike",
                            "unlink": "Unlink",
                            "heading": "Heading",
                            "preview": "Preview",
                            "heading1": "H1",
                            "heading2": "H2",
                            "heading3": "H3",
                            "heading4": "H4",
                            "imageUrl": "Image URL",
                            "alignLeft": "Left",
                            "codeBlock": "Code Block",
                            "paragraph": "Paragraph",
                            "textColor": "Text Color",
                            "underline": "Underline",
                            "alignRight": "Right",
                            "blockquote": "Quote",
                            "bulletList": "Bullets",
                            "alignCenter": "Center",
                            "imageUpload": "Upload Image",
                            "orderedList": "Numbering",
                            "highlightColor": "Highlight Color"
                        },
                        "placeholder": "Please enter content..."
                    },
                    "captcha": {
                        "alt": "Supports img tag src attribute value",
                        "title": "Please complete the security verification",
                        "confirm": "Confirm",
                        "clickInOrder": "Please click in order",
                        "pointAriaLabel": "Click point",
                        "confirmAriaLabel": "Confirm selection",
                        "refreshAriaLabel": "Refresh captcha",
                        "sliderDefaultText": "Slider and drag",
                        "sliderSuccessText": "Passed",
                        "sliderRotateFailTip": "Validation failed",
                        "sliderRotateDefaultTip": "Click picture to refresh",
                        "sliderRotateSuccessTip": "Validation successful, time {0} seconds",
                        "sliderTranslateFailTip": "Validation failed",
                        "sliderTranslateDefaultTip": "Click picture to refresh",
                        "sliderTranslateSuccessTip": "Validation successful, time {0} seconds"
                    },
                    "widgets": {
                        "qa": "Q&A",
                        "search": {
                            "close": "Close",
                            "title": "Search",
                            "recent": "Search History",
                            "select": "Select",
                            "history": "History",
                            "results": "Search results",
                            "navigate": "Navigate",
                            "noRecent": "No Search History",
                            "noResults": "No Search Results Found",
                            "removeHistory": "Remove {value} from history",
                            "searchNavigate": "Search Navigation"
                        },
                        "setting": "Settings",
                        "viewAll": "View All Messages",
                        "document": "Document",
                        "timezone": {
                            "setSuccess": "Timezone set successfully",
                            "setTimezone": "Set Timezone"
                        },
                        "logoutTip": "Do you want to logout?",
                        "lockScreen": {
                            "entry": "Enter the system",
                            "title": "Lock Screen",
                            "unlock": "Click to unlock",
                            "password": "Password",
                            "backToLogin": "Back to login",
                            "placeholder": "Please enter password",
                            "screenButton": "Lock",
                            "errorPasswordTip": "Password error, please re-enter"
                        },
                        "markAllAsRead": "Mark all as read",
                        "notifications": "Notifications",
                        "checkUpdatesTitle": "New Version Available",
                        "clearNotifications": "Clear",
                        "checkUpdatesDescription": "Click to refresh and get the latest version"
                    },
                    "fallback": {
                        "http": {
                            "notFound": "Not Found. The requested resource could not be found.",
                            "forbidden": "Forbidden. You do not have permission to access this resource.",
                            "badRequest": "Bad Request. Please check your input and try again.",
                            "networkError": "A network error occurred. Please check your internet connection and try again.",
                            "unauthorized": "Unauthorized. Please log in to continue.",
                            "requestTimeout": "The request timed out. Please try again later.",
                            "internalServerError": "Internal Server Error. Something went wrong on our end. Please try again later."
                        },
                        "offline": "Offline Page",
                        "forbidden": "Oops! Access Denied",
                        "comingSoon": "Coming Soon",
                        "offlineError": "Oops! Network Error",
                        "pageNotFound": "Oops! Page Not Found",
                        "forbiddenDesc": "Sorry, but you don''t have permission to access this page.",
                        "internalError": "Oops! Something Went Wrong",
                        "offlineErrorDesc": "Sorry, can''t connect to the internet. Check your connection.",
                        "pageNotFoundDesc": "Sorry, we couldn''t find the page you were looking for.",
                        "internalErrorDesc": "Sorry, but the server encountered an error."
                    },
                    "formRules": {
                        "length": "{0} must be {1} characters long",
                        "required": "Please enter {0}",
                        "maxLength": "{0} can be at most {1} characters",
                        "minLength": "{0} must be at least {1} characters",
                        "sizeLimit": "The file size cannot exceed {0}MB",
                        "startWith": "{0} must start with `{1}`",
                        "invalidURL": "Please input a valid URL",
                        "alreadyExists": "{0} `{1}` already exists",
                        "previewWarning": "Unable to open the file, there is no available URL or preview address",
                        "selectRequired": "Please select {0}"
                    },
                    "iconPicker": {
                        "search": "Search icon...",
                        "placeholder": "Select an icon"
                    },
                    "jsonViewer": {
                        "copy": "Copy",
                        "copied": "Copied"
                    },
                    "actionTitle": {
                        "edit": "Modify {0}",
                        "view": "View {0}",
                        "create": "Create {0}",
                        "delete": "Delete {0}"
                    },
                    "placeholder": {
                        "input": "Please enter",
                        "select": "Please select",
                        "upload": "Click to upload"
                    },
                    "actionMessage": {
                        "deleting": "Deleting {0} ...",
                        "deleteConfirm": "Are you sure to delete {0}?",
                        "deleteSuccess": "{0} deleted successfully",
                        "operationFailed": "Operation failed",
                        "operationSuccess": "Operation succeeded"
                    }
                },
                "app": {
                    "errors": {
                        "failed": "Request failed. Please try again later.",
                        "dataLoad": "Unable to load data. Please retry.",
                        "serverLost": "The server seems to be lost~",
                        "statisticsLoad": "Unable to load statistics. Please retry."
                    },
                    "captcha": {
                        "start": "Click to start verification",
                        "failed": "Incorrect order. Try again",
                        "passed": "Verification passed",
                        "checking": "Verifying…",
                        "progress": "Verification in progress…",
                        "additional": "Additional verification",
                        "sliderFailed": "Verification failed. Drag a little slower to the far right and try again",
                        "sliderIncomplete": "Drag the slider all the way to the right"
                    },
                    "journey": {
                        "title": "Welcome ashore",
                        "entering": "Signed in. Taking you in…",
                        "description": "Sign in to Go Cinch and pick up where you left off"
                    },
                    "register": {
                        "success": "Account created successfully. You can sign in after administrator approval.",
                        "usernameExists": "Username already exists"
                    },
                    "validation": {
                        "password": "Password is required",
                        "username": "Username is required",
                        "verification": "Please complete the verification first",
                        "currentPassword": "Current password is required",
                        "passwordRequired": "Password is required",
                        "credentialsRequired": "Username and password are required",
                        "passwordChangeRequired": "Current and new passwords are required"
                    },
                    "resetPassword": {
                        "title": "Reset password",
                        "logout": "Sign out",
                        "submit": "Reset and continue",
                        "success": "Password reset successfully",
                        "description": "Before continuing after your first login or an administrator reset, choose a new password different from your current one."
                    }
                },
                "msg": {
                    "all": "Everyone",
                    "hint": "Read notifications and manage your inbox.",
                    "next": "Next",
                    "page": "Page {page}, {total} messages",
                    "read": "Read",
                    "send": "Send message",
                    "sent": "Message sent",
                    "type": "Message type",
                    "view": "View",
                    "close": "Close",
                    "count": "{count} unread",
                    "empty": "No messages",
                    "error": "Unable to load. Please retry.",
                    "inbox": "My messages",
                    "plain": "Plain",
                    "retry": "Retry",
                    "saved": "Done",
                    "scope": "Audience",
                    "style": "List style",
                    "title": "Title",
                    "cancel": "Cancel",
                    "delete": "Delete",
                    "detail": "Message details",
                    "expiry": "Expires at",
                    "manage": "Messages",
                    "normal": "Normal",
                    "notice": "Notice",
                    "select": "Select",
                    "sender": "Sender ID",
                    "system": "System",
                    "unread": "Unread",
                    "actions": "Actions",
                    "columns": "Visible fields",
                    "compact": "Compact",
                    "confirm": "Confirm",
                    "content": "Content",
                    "density": "Density",
                    "expired": "Expired",
                    "feedAll": "All",
                    "feedEnd": "All messages loaded",
                    "history": "Message history",
                    "keyword": "Keyword",
                    "loading": "Loading messages…",
                    "preview": "Message preview",
                    "readAll": "Mark all as read",
                    "refresh": "Refresh",
                    "striped": "Striped",
                    "unsaved": "Discard the unsent message?",
                    "allTypes": "All types",
                    "markRead": "Mark as read",
                    "noExpiry": "No expiry",
                    "pageSize": "Page size",
                    "previous": "Previous",
                    "targeted": "Selected users",
                    "allScopes": "All audiences",
                    "allStatus": "All statuses",
                    "feedOrder": "Newest first",
                    "feedShown": "Showing {count} of {total}",
                    "published": "Published at",
                    "selectAll": "Select all",
                    "splitView": "Split view",
                    "tableView": "Table view",
                    "feedCenter": "Message center",
                    "feedExpand": "Expand message",
                    "fullscreen": "Fullscreen",
                    "manageHint": "Publish broadcasts or targeted messages and review sent messages.",
                    "recipients": "Recipients",
                    "searchUser": "Search username",
                    "titleError": "Enter a title of up to 200 characters",
                    "detailError": "Unable to load message details. Please retry.",
                    "expiryError": "Choose a valid future time",
                    "feedFilters": "Filter messages",
                    "feedNoMatch": "No matching messages",
                    "feedUpdates": "Messages updated. Refresh",
                    "contentError": "Enter content of up to 20000 characters",
                    "deleteGlobal": "Delete for everyone",
                    "feedCollapse": "Collapse",
                    "feedLoadMore": "Scroll down or load more",
                    "feedNoUnread": "No unread messages",
                    "noPermission": "You do not have permission to view sent messages",
                    "noRecipients": "No recipients",
                    "recipientIDs": "Recipient IDs",
                    "summaryError": "Message statistics could not load. Published records remain available.",
                    "activeFilters": "Active filters",
                    "broadcastHint": "Visible only to users who registered before the message was published.",
                    "deleteConfirm": "Delete this message from your inbox?",
                    "messageFilter": "Message: {title}",
                    "selectPreview": "Select a message to preview its content",
                    "viewRecipient": "View user {username}",
                    "deleteSelected": "Delete selected",
                    "publishedCount": "Published messages",
                    "recipientCount": "{count} recipients",
                    "recipientError": "Choose 1–1000 recipients",
                    "searchMessages": "Search message titles or content",
                    "feedLoadingMore": "Loading earlier messages…",
                    "latestPublished": "Latest publication",
                    "selectedMessage": "Selected message",
                    "deletedRecipient": "Deleted user",
                    "feedSelectLoaded": "Select loaded messages",
                    "focusUnavailable": "This message is unavailable or does not match the current filters.",
                    "publishedRecords": "Published records",
                    "typeDistribution": "{notice} notices and {system} system messages",
                    "scopeDistribution": "{all} broadcasts and {targeted} targeted messages",
                    "clearMessageFilter": "Clear message filter",
                    "clearPreviewConfirm": "Delete the {count} messages currently shown in notifications from your inbox?",
                    "deleteGlobalConfirm": "This permanently removes the message for every recipient. Delete it?",
                    "readSelectedConfirm": "Mark selected messages as read?",
                    "deleteSelectedConfirm": "Delete selected messages from your inbox?",
                    "recipientsUnavailable": "Recipient information unavailable",
                    "deleteSelectedGlobalConfirm": "Delete selected messages for every recipient?"
                },
                "page": {
                    "auth": {
                        "login": "Login",
                        "profile": "Profile",
                        "register": "Register",
                        "codeLogin": "Code Login",
                        "qrcodeLogin": "Qr Code Login"
                    },
                    "profile": {
                        "tabs": {
                            "changePassword": "Change Password",
                            "basicInformation": "Basic Information"
                        },
                        "title": "Profile",
                        "fields": {
                            "role": "Role",
                            "username": "Username"
                        },
                        "password": {
                            "success": "Password updated. Please sign in again on all devices.",
                            "newPassword": "New Password",
                            "oldPassword": "Current Password",
                            "confirmPassword": "Confirm Password",
                            "passwordMismatch": "The passwords do not match",
                            "newPasswordPlaceholder": "Please enter the new password",
                            "oldPasswordPlaceholder": "Please enter your current password",
                            "confirmPasswordPlaceholder": "Please enter the new password again"
                        }
                    },
                    "dashboard": {
                        "title": "Dashboard",
                        "overview": "Overview",
                        "workspace": "Workspace",
                        "overviewPage": {
                            "tabs": {
                                "monthlyVisits": "Monthly Visits",
                                "trafficTrends": "Traffic Trends"
                            },
                            "cards": {
                                "visitCount": "Visit Overview",
                                "businessMix": "Business Mix",
                                "visitSource": "Visit Sources"
                            },
                            "charts": {
                                "web": "Web",
                                "month": "Month {month}",
                                "other": "Other",
                                "trend": "Trend",
                                "visit": "Visits",
                                "client": "Desktop App",
                                "direct": "Direct",
                                "mobile": "Mobile",
                                "remote": "Remote",
                                "tablet": "Tablet",
                                "thirdParty": "Third Party",
                                "businessMix": "Business Mix",
                                "outsourcing": "Outsourcing",
                                "visitSource": "Visit Sources",
                                "affiliateAds": "Affiliate Ads",
                                "searchEngine": "Search Engines",
                                "customization": "Customization",
                                "emailMarketing": "Email Marketing",
                                "technicalSupport": "Technical Support"
                            },
                            "metrics": {
                                "usage": "Usage",
                                "users": "Users",
                                "visits": "Visits",
                                "downloads": "Downloads",
                                "totalUsage": "Total Usage",
                                "totalUsers": "Total Users",
                                "totalVisits": "Total Visits",
                                "totalDownloads": "Total Downloads"
                            }
                        },
                        "workspacePage": {
                            "stats": {
                                "teams": "Teams",
                                "todos": "To-dos",
                                "projects": "Projects"
                            },
                            "todos": {
                                "uiIssue": {
                                    "title": "Fix UI Display Issues",
                                    "content": "Fix reported UI display issues and ensure consistent rendering across browsers."
                                },
                                "security": {
                                    "title": "Security Review",
                                    "content": "Run a security review to identify vulnerabilities and unauthorized access."
                                },
                                "codeReview": {
                                    "title": "Review Frontend Commits",
                                    "content": "Review recent frontend commits in the Git repository for quality and consistency."
                                },
                                "performance": {
                                    "title": "Optimize System Performance",
                                    "content": "Inspect and optimize system performance to reduce CPU usage."
                                },
                                "dependencies": {
                                    "title": "Update Dependencies",
                                    "content": "Update all npm dependencies in the project to their latest compatible versions."
                                }
                            },
                            "groups": {
                                "web": "Web Team",
                                "algorithm": "Algorithm Team",
                                "openSource": "Open Source Team",
                                "technology": "Technology Team",
                                "architecture": "Architecture Team"
                            },
                            "header": {
                                "weather": "Sunny today, 20°C–32°C.",
                                "greeting": "Good morning, {name}. Ready to start your day!"
                            },
                            "people": {
                                "evan": "Evan",
                                "jack": "Jack",
                                "chris": "Chris",
                                "peter": "Peter",
                                "william": "William"
                            },
                            "trends": {
                                "dates": {
                                    "justNow": "Just now",
                                    "oneDayAgo": "1 day ago",
                                    "oneHourAgo": "1 hour ago",
                                    "oneWeekAgo": "1 week ago",
                                    "twoDaysAgo": "2 days ago",
                                    "threeDaysAgo": "3 days ago"
                                },
                                "items": {
                                    "postedUpdate": "Posted a personal update",
                                    "pushedGithub": "Pushed code to GitHub",
                                    "publishedVite": "Published \"How to write a Vite plugin\"",
                                    "closedRunIssue": "Closed the issue \"How to run the project\"",
                                    "createdProject": "Created the Vue project in the Open Source Team",
                                    "followedWilliam": "Followed William",
                                    "publishedAdminVben": "Published \"How to use go cinch\"",
                                    "repliedOptimization": "Replied to Jack''s question \"How can I optimize a project?\""
                                }
                            },
                            "projects": {
                                "vueQuote": "Who you are today shapes who you become tomorrow.",
                                "htmlQuote": "No talent matters more than hard work.",
                                "reactQuote": "A healthy body is the foundation for achieving your goals.",
                                "githubQuote": "Do not wait for opportunity. Create it.",
                                "angularQuote": "Passion and determination can overcome any obstacle.",
                                "javascriptQuote": "The path is made by walking, not by imagining."
                            },
                            "sections": {
                                "todos": "To-dos",
                                "projects": "Projects",
                                "latestActivity": "Latest Activity",
                                "quickNavigation": "Quick Navigation"
                            },
                            "navigation": {
                                "home": "Home",
                                "charts": "Charts",
                                "dashboard": "Dashboard",
                                "components": "Components",
                                "accessManagement": "Access Management",
                                "systemManagement": "System Management"
                            }
                        }
                    },
                    "notifications": {
                        "dates": {
                            "justNow": "Just now",
                            "fixedDate": "2024-01-01",
                            "oneDayAgo": "1 day ago",
                            "threeHoursAgo": "3 hours ago"
                        },
                        "titles": {
                            "replied": "Zhu Pianyou replied to you",
                            "commented": "Qu Lili commented on your post",
                            "externalLink": "Open external link example",
                            "todoReminder": "To-do reminder",
                            "weeklyReports": "Received 14 new weekly reports",
                            "workspaceLink": "Open workspace example"
                        },
                        "description": "This is the notification description."
                    }
                },
                "audit": {
                    "id": "Event ID",
                    "ip": "Source IP",
                    "all": "All",
                    "p95": "P95 duration",
                    "less": "Less",
                    "menu": "Audit",
                    "more": "More",
                    "path": "Request path",
                    "peak": "Peak: {day} {hour}:00 · {count} events",
                    "actor": "Actor",
                    "empty": "No events in this range",
                    "login": "Login logs",
                    "reset": "Reset",
                    "retry": "Retry",
                    "total": "Total events",
                    "trend": "Event trend",
                    "client": "Client",
                    "copied": "Copied",
                    "events": "Event records",
                    "export": "Export CSV",
                    "failed": "Failures",
                    "method": "HTTP method",
                    "module": "Module",
                    "period": "Time range",
                    "search": "Search",
                    "status": "Result",
                    "actions": {
                        "create": "Create {module}",
                        "remove": "Delete {module}",
                        "update": "Update {module}"
                    },
                    "average": "Average duration",
                    "dayView": "Day",
                    "details": "Details",
                    "failure": "Failure",
                    "heatmap": "Login activity heatmap",
                    "insight": "Period insights",
                    "latency": "Response time",
                    "modules": {
                        "msg": "Messages",
                        "auth": "Authentication",
                        "role": "Roles",
                        "user": "Users",
                        "action": "Actions",
                        "session": "Session",
                        "dictionary": "Dictionaries",
                        "user-group": "User groups"
                    },
                    "noTrace": "Not recorded",
                    "period1": "Last 24 hours",
                    "period7": "Last 7 days",
                    "request": "Request",
                    "success": "Success",
                    "user_id": "Actor ID",
                    "business": "Operation",
                    "continue": "Continue investigating",
                    "copyPath": "Copy request path",
                    "heatCell": "{day} {hour}:00, {count} logins, {failed} failures",
                    "heatPeak": "Peak: {time} · {count} logins",
                    "heatView": "Heatmap",
                    "hourView": "Hour",
                    "noAccess": "You do not have permission to view these audit events.",
                    "noParams": "No parameters recorded",
                    "outcomes": "Outcome distribution",
                    "period30": "Last 30 days",
                    "samePath": "Same request",
                    "trace_id": "Trace ID",
                    "username": "Account",
                    "weekday0": "Mon",
                    "weekday1": "Tue",
                    "weekday2": "Wed",
                    "weekday3": "Thu",
                    "weekday4": "Fri",
                    "weekday5": "Sat",
                    "weekday6": "Sun",
                    "anonymous": "Unknown account",
                    "copyError": "Unable to copy. Select and copy the text.",
                    "copyTrace": "Copy trace ID",
                    "errorInfo": "Error information",
                    "loadError": "Unable to load audit events. Please retry.",
                    "localTime": "Selected timezone",
                    "noSources": "No source IPs recorded",
                    "openEvent": "Open event {id}",
                    "operation": "Operation logs",
                    "sameActor": "Same actor",
                    "timeFirst": "Time first",
                    "trendView": "Trend",
                    "uniqueIPs": "Source IPs",
                    "writeOnly": "Business writes",
                    "allModules": "All modules",
                    "allPeriods": "All time buckets",
                    "allRecords": "Latest first",
                    "byDuration": "By duration",
                    "error_code": "Error code",
                    "httpStatus": "HTTP status",
                    "loginCount": "Login attempts",
                    "noFailures": "No failures in this scope",
                    "paramsHint": "Sensitive values are hidden. File contents are replaced with file.",
                    "resourceId": "Target ID",
                    "restoreAll": "Clear filters",
                    "searchHint": "Account, IP, path or trace ID",
                    "topEntries": "Top {count}",
                    "user_agent": "User agent",
                    "closeDetail": "Close details",
                    "duration_ms": "Duration",
                    "eventDetail": "Event details",
                    "exportError": "Export failed. Try again or narrow the range to 10,000 events.",
                    "hasFailures": "Contains failures",
                    "occurred_at": "Time",
                    "otherClient": "Other client",
                    "recordCount": "{count} records",
                    "recordOrder": "Record order",
                    "sourceFocus": "Active sources",
                    "successRate": "Success rate",
                    "uniqueUsers": "Accounts",
                    "choosePeriod": "Select time bucket",
                    "expandDetail": "Expand details",
                    "failureFirst": "Failures first",
                    "heatInterval": "{start} — {end}, {count} logins, {failed} failures",
                    "heatmapLevel": "Heatmap granularity",
                    "loginAttempt": "Login attempt",
                    "loginRecords": "Login records",
                    "resourceType": "Target type",
                    "selectRecord": "Select an event",
                    "filterApplied": "Account filter applied: {value}",
                    "loginActivity": "Login activity",
                    "loginAttempts": "Login attempts",
                    "operationType": "Operation type",
                    "requestFailed": "The request did not complete successfully.",
                    "requestParams": "Request parameters",
                    "failedAccounts": "Accounts with failures",
                    "operationCount": "Operations",
                    "operationTypes": {
                        "read": "Mark as read",
                        "create": "Create",
                        "delete": "Delete",
                        "logout": "Log out",
                        "update": "Update",
                        "read_all": "Mark all as read",
                        "register": "Register",
                        "reset_password": "Reset password",
                        "change_password": "Change password",
                        "delete_personal": "Delete from inbox"
                    },
                    "responseParams": "Response parameters",
                    "selectedPeriod": "Selected period",
                    "activityLoading": "Loading activity",
                    "failureDominant": "More failures",
                    "hourUnavailable": "{hour}:00, this hour is not available",
                    "operationRecords": "Operation records",
                    "repeatedFailures": "Repeated failures",
                    "timeDistribution": "By time",
                    "operationActivity": "Operation activity",
                    "moduleDistribution": "Operations by module",
                    "noRepeatedFailures": "No repeated failures"
                },
                "demos": {
                    "antd": "Ant Design Vue",
                    "vben": {
                        "about": "About",
                        "antdv": "Ant Design Vue Version",
                        "title": "Project",
                        "tdesign": "TDesign Vue Version",
                        "document": "Document",
                        "naive-ui": "Naive UI Version",
                        "antdv-next": "Antdv Next Version",
                        "element-plus": "Element Plus Version"
                    },
                    "title": "Demos"
                },
                "common": {
                    "no": "No",
                    "yes": "Yes",
                    "back": "Back",
                    "edit": "Edit",
                    "more": "More",
                    "login": "Login",
                    "query": "Search",
                    "reset": "Reset",
                    "cancel": "Cancel",
                    "create": "Create",
                    "delete": "Delete",
                    "detail": "Detail",
                    "logout": "Logout",
                    "noData": "No Data",
                    "prompt": "Prompt",
                    "search": "Search",
                    "confirm": "Confirm",
                    "enabled": "Enabled",
                    "notShow": "Don''t Show",
                    "refresh": "Refresh",
                    "disabled": "Disabled",
                    "backToHome": "Back To Home",
                    "hideSearchPanel": "Hide search panel",
                    "showSearchPanel": "Show search panel"
                },
                "groups": {
                    "admin:system:msg": "Message",
                    "admin:audit:login": "Login audit",
                    "admin:system:role": "Role",
                    "admin:system:user": "User",
                    "admin:system:action": "Action",
                    "admin:system:general": "General",
                    "admin:audit:operation": "Operation audit",
                    "admin:system:dictionary": "Dictionary",
                    "admin:system:user-group": "User group",
                    "admin:dashboard:overview": "Dashboard",
                    "admin:dashboard:workspace": "Workspace",
                    "admin:system:permission-admin": "Manage permissions",
                    "admin:system:developer": "Developer"
                },
                "scopes": {
                    "h5": "Mobile H5",
                    "app": "Application",
                    "mini": "Mini Program",
                    "admin": "Admin Console"
                },
                "system": {
                    "menu": {
                        "roles": "Roles",
                        "users": "Users",
                        "groups": "User Groups",
                        "system": "System",
                        "actions": "Actions",
                        "dictionary": "Dictionary"
                    },
                    "role": {
                        "title": "Role Management"
                    },
                    "user": {
                        "lock": "Lock",
                        "title": "User Management",
                        "entity": "User",
                        "reason": "Reason",
                        "reject": "Reject",
                        "review": "Review Registration",
                        "unlock": "Unlock",
                        "approve": "Approve",
                        "decision": "Decision",
                        "lockType": "Lock Type",
                        "lockLabel": "Lock:",
                        "lockTitle": "Lock User “{name}”",
                        "lockUntil": "Until a specific time",
                        "permanent": "Permanent",
                        "lockedUntil": "Locked Until",
                        "untilPrefix": "Until",
                        "keepPassword": "Leave blank to keep the current password",
                        "unlockPassword": "Unlock Password Change",
                        "rejectionPlaceholder": "Enter the registration rejection reason"
                    },
                    "hints": {
                        "createGroup": "Create “{name}”"
                    },
                    "table": {
                        "loose": "Loose",
                        "style": "Table Style",
                        "total": "{count} records",
                        "sticky": "Sticky Header",
                        "columns": "Columns",
                        "compact": "Compact",
                        "default": "Default",
                        "density": "Density",
                        "striped": "Striped Rows",
                        "bordered": "Bordered",
                        "fullscreen": "Fullscreen",
                        "exitFullscreen": "Exit Fullscreen",
                        "visibleColumns": "Visible Columns"
                    },
                    "action": {
                        "title": "Action Management",
                        "entity": "Action"
                    },
                    "common": {
                        "edit": "Edit",
                        "less": "Less",
                        "more": "More",
                        "enter": "Enter {field}",
                        "reset": "Reset",
                        "create": "Create",
                        "delete": "Delete",
                        "search": "Search",
                        "select": "Select {field}",
                        "actions": "Actions",
                        "refresh": "Refresh",
                        "unknown": "Unknown",
                        "editTitle": "Edit {entity}",
                        "selectAll": "Select All",
                        "selection": "Selection",
                        "createTitle": "Create {entity}",
                        "deleteSelected": "Delete Selected"
                    },
                    "fields": {
                        "id": "ID",
                        "code": "Code",
                        "name": "Name",
                        "role": "Role",
                        "word": "Word",
                        "group": "Group",
                        "status": "Status",
                        "enabled": "Enabled",
                        "members": "Members",
                        "category": "Category",
                        "menuPath": "Menu Path",
                        "metadata": "Metadata",
                        "password": "Password",
                        "resource": "Resource",
                        "userCode": "User Code",
                        "username": "Username",
                        "createdAt": "Created At",
                        "menuPaths": "Menu Paths",
                        "updatedAt": "Updated At",
                        "actionCode": "Action Code",
                        "description": "Description",
                        "newPassword": "New Password",
                        "permissions": "Permissions",
                        "dictionaryKey": "Dictionary Key",
                        "resourceRules": "Resource Rules",
                        "dictionaryValue": "Value (JSON)",
                        "buttonPermission": "Button Permission"
                    },
                    "status": {
                        "active": "Active",
                        "locked": "Locked",
                        "pending": "Pending"
                    },
                    "confirm": {
                        "delete": "Delete “{name}”?",
                        "unlock": "Unlock “{name}”?",
                        "deleteSelected": "Delete selected records?",
                        "unlockPassword": "Unlock self-service password changes for “{name}”?",
                        "deleteDescription": "The selected {count} records will be deleted."
                    },
                    "enabled": {
                        "no": "Disabled",
                        "yes": "Enabled"
                    },
                    "category": {
                        "jwt": "JWT",
                        "permission": "Permission"
                    },
                    "messages": {
                        "locked": "User locked successfully",
                        "created": "Created successfully",
                        "deleted": "Deleted successfully",
                        "updated": "Updated successfully",
                        "approved": "Registration approved",
                        "rejected": "Registration rejected",
                        "unlocked": "User unlocked successfully",
                        "noChanges": "No changes to save",
                        "passwordUnlocked": "Self-service password change unlocked"
                    },
                    "userGroup": {
                        "title": "User Group Management",
                        "entity": "User Group"
                    },
                    "dictionary": {
                        "title": "Dictionary Management",
                        "entity": "Dictionary"
                    },
                    "validation": {
                        "json": "Enter valid JSON",
                        "required": "{field} is required",
                        "futureLock": "Please select a future lock expiration time",
                        "dictionaryKey": "Use uppercase letters, digits, and underscore-separated segments",
                        "rejectionRequired": "Please enter a rejection reason"
                    },
                    "suggestions": {
                        "role": "Search by role name or key",
                        "user": "Search by username or code",
                        "action": "Search by permission name or key",
                        "backend": "Backend results",
                        "history": "History",
                        "searching": "Searching…",
                        "actionGroup": "Search or create an action group",
                        "removeHistory": "Remove {value} from history"
                    }
                },
                "actions": {
                    "admin:read:system:msg": "View message",
                    "admin:read:audit:login": "View login audit",
                    "admin:read:system:role": "View role",
                    "admin:read:system:user": "View user",
                    "admin:create:system:msg": "Send message",
                    "admin:delete:system:msg": "Delete message",
                    "admin:create:system:role": "Create role",
                    "admin:create:system:user": "Create user",
                    "admin:delete:system:role": "Delete role",
                    "admin:delete:system:user": "Delete user",
                    "admin:read:system:action": "View permission",
                    "admin:update:system:role": "Edit role",
                    "admin:update:system:user": "Edit user",
                    "admin:create:system:action": "Create permission",
                    "admin:delete:system:action": "Delete permission",
                    "admin:read:audit:operation": "View operation audit",
                    "admin:update:system:action": "Edit permission",
                    "admin:read:system:dictionary": "View dictionary",
                    "admin:read:system:user-group": "View user group",
                    "admin:read:dashboard:overview": "View dashboard",
                    "admin:create:system:dictionary": "Create dictionary",
                    "admin:create:system:user-group": "Create user group",
                    "admin:delete:system:dictionary": "Delete dictionary",
                    "admin:delete:system:user-group": "Delete user group",
                    "admin:read:dashboard:workspace": "Workspace Read",
                    "admin:update:system:dictionary": "Edit dictionary",
                    "admin:update:system:user-group": "Edit user group",
                    "admin:all:system:general": "All permissions",
                    "admin:default:system:general": "Default permissions",
                    "admin:grant:system:permission-admin": "Manage permissions",
                    "admin:update:system:developer": "Manage developer configuration"
                },
                "buttons": {
                    "admin:btn:msg:inbox:query": "Search",
                    "admin:btn:system:msg:send": "Send message",
                    "admin:btn:msg:inbox:delete": "Delete",
                    "admin:btn:system:msg:query": "Search",
                    "admin:btn:system:role:edit": "Edit",
                    "admin:btn:system:user:edit": "Edit",
                    "admin:btn:system:user:lock": "Lock",
                    "admin:btn:audit:login:query": "Search",
                    "admin:btn:msg:inbox:refresh": "Refresh",
                    "admin:btn:system:msg:delete": "Delete",
                    "admin:btn:system:role:query": "Search",
                    "admin:btn:system:user:query": "Search",
                    "admin:btn:msg:inbox:read-all": "Mark all as read",
                    "admin:btn:system:action:edit": "Edit",
                    "admin:btn:system:msg:refresh": "Refresh",
                    "admin:btn:system:role:create": "Create",
                    "admin:btn:system:role:delete": "Delete",
                    "admin:btn:system:user:create": "Create",
                    "admin:btn:system:user:delete": "Delete",
                    "admin:btn:system:user:unlock": "Unlock",
                    "admin:btn:audit:login:refresh": "Refresh",
                    "admin:btn:msg:inbox:mark-read": "Mark as read",
                    "admin:btn:system:action:query": "Search",
                    "admin:btn:system:role:refresh": "Refresh",
                    "admin:btn:system:user:approve": "Approve",
                    "admin:btn:system:user:refresh": "Refresh",
                    "admin:btn:system:action:create": "Create",
                    "admin:btn:system:action:delete": "Delete",
                    "admin:btn:audit:operation:query": "Search",
                    "admin:btn:msg:inbox:view-detail": "Details",
                    "admin:btn:system:action:refresh": "Refresh",
                    "admin:btn:system:dictionary:edit": "Edit",
                    "admin:btn:system:msg:reset-query": "Reset",
                    "admin:btn:system:msg:view-detail": "Details",
                    "admin:btn:system:user-group:edit": "Edit",
                    "admin:btn:audit:login:reset-query": "Reset",
                    "admin:btn:audit:login:view-detail": "Details",
                    "admin:btn:audit:operation:refresh": "Refresh",
                    "admin:btn:system:dictionary:query": "Search",
                    "admin:btn:system:role:reset-query": "Reset",
                    "admin:btn:system:role:view-detail": "Details",
                    "admin:btn:system:user-group:query": "Search",
                    "admin:btn:system:user:reset-query": "Reset",
                    "admin:btn:system:user:view-detail": "Details",
                    "admin:btn:system:dictionary:create": "Create",
                    "admin:btn:system:dictionary:delete": "Delete",
                    "admin:btn:system:user-group:create": "Create",
                    "admin:btn:system:user-group:delete": "Delete",
                    "admin:btn:audit:login:export-report": "Export CSV",
                    "admin:btn:msg:inbox:delete-selected": "Delete Selected",
                    "admin:btn:system:action:reset-query": "Reset",
                    "admin:btn:system:action:view-detail": "Details",
                    "admin:btn:system:dictionary:refresh": "Refresh",
                    "admin:btn:system:user-group:refresh": "Refresh",
                    "admin:btn:system:msg:delete-selected": "Delete Selected",
                    "admin:btn:audit:operation:reset-query": "Reset",
                    "admin:btn:audit:operation:view-detail": "Details",
                    "admin:btn:system:role:delete-selected": "Delete Selected",
                    "admin:btn:system:user:delete-selected": "Delete Selected",
                    "admin:btn:system:user:unlock-password": "Unlock Password Change",
                    "admin:btn:msg:inbox:mark-selected-read": "Mark as read",
                    "admin:btn:audit:operation:export-report": "Export CSV",
                    "admin:btn:system:action:delete-selected": "Delete Selected",
                    "admin:btn:system:dictionary:reset-query": "Reset",
                    "admin:btn:system:dictionary:view-detail": "Details",
                    "admin:btn:system:user-group:add-members": "Add members",
                    "admin:btn:system:user-group:reset-query": "Reset",
                    "admin:btn:system:user-group:view-detail": "Details",
                    "admin:btn:system:user-group:view-members": "View members",
                    "admin:btn:account:profile:change-password": "Change password",
                    "admin:btn:system:user-group:remove-members": "Remove",
                    "admin:btn:system:dictionary:delete-selected": "Delete Selected",
                    "admin:btn:system:user-group:delete-selected": "Delete Selected",
                    "admin:btn:system:developer:compare-online": "Compare with online",
                    "admin:btn:system:developer:refresh": "Refresh online catalog",
                    "admin:btn:system:developer:publish": "Publish catalog",
                    "admin:btn:system:action:create-group": "Create permission group",
                    "admin:btn:system:action:reorder-groups": "Reorder permission groups",
                    "admin:btn:system:action:delete-group": "Delete permission group"
                },
                "profile": {
                    "updatePassword": "Update Password",
                    "updateBasicProfile": "Update Basic Profile"
                },
                "features": {
                    "admin:system:msg:send": "Send message",
                    "admin:system:msg:query": "Search",
                    "admin:system:role:edit": "Edit",
                    "admin:system:user:edit": "Edit",
                    "admin:system:user:lock": "Lock",
                    "admin:audit:login:query": "Search",
                    "admin:system:msg:delete": "Delete",
                    "admin:system:role:query": "Search",
                    "admin:system:user:query": "Search",
                    "admin:system:action:edit": "Edit",
                    "admin:system:msg:refresh": "Refresh",
                    "admin:system:role:create": "Create",
                    "admin:system:role:delete": "Delete",
                    "admin:system:user:create": "Create",
                    "admin:system:user:delete": "Delete",
                    "admin:system:user:unlock": "Unlock",
                    "admin:audit:login:refresh": "Refresh",
                    "admin:system:action:query": "Search",
                    "admin:system:role:refresh": "Refresh",
                    "admin:system:user:approve": "Approve",
                    "admin:system:user:refresh": "Refresh",
                    "admin:system:action:create": "Create",
                    "admin:system:action:delete": "Delete",
                    "admin:audit:operation:query": "Search",
                    "admin:system:action:refresh": "Refresh",
                    "admin:default:session:access": "Session and account operations",
                    "admin:system:dictionary:edit": "Edit",
                    "admin:system:msg:reset-query": "Reset",
                    "admin:system:msg:view-detail": "Details",
                    "admin:system:user-group:edit": "Edit",
                    "admin:audit:login:reset-query": "Reset",
                    "admin:audit:login:view-detail": "Details",
                    "admin:audit:operation:refresh": "Refresh",
                    "admin:system:dictionary:query": "Search",
                    "admin:system:role:reset-query": "Reset",
                    "admin:system:role:view-detail": "Details",
                    "admin:system:user-group:query": "Search",
                    "admin:system:user:reset-query": "Reset",
                    "admin:system:user:view-detail": "Details",
                    "admin:msg:inbox:default-access": "Basic access",
                    "admin:system:dictionary:create": "Create",
                    "admin:system:dictionary:delete": "Delete",
                    "admin:system:user-group:create": "Create",
                    "admin:system:user-group:delete": "Delete",
                    "admin:audit:login:export-report": "Export CSV",
                    "admin:system:action:reset-query": "Reset",
                    "admin:system:action:view-detail": "Details",
                    "admin:system:dictionary:refresh": "Refresh",
                    "admin:system:user-group:refresh": "Refresh",
                    "admin:system:msg:delete-selected": "Delete Selected",
                    "admin:audit:operation:reset-query": "Reset",
                    "admin:audit:operation:view-detail": "Details",
                    "admin:system:role:delete-selected": "Delete Selected",
                    "admin:system:user:delete-selected": "Delete Selected",
                    "admin:system:user:unlock-password": "Unlock Password Change",
                    "admin:audit:operation:export-report": "Export CSV",
                    "admin:system:action:delete-selected": "Delete Selected",
                    "admin:system:dictionary:reset-query": "Reset",
                    "admin:system:dictionary:view-detail": "Details",
                    "admin:system:user-group:add-members": "Add members",
                    "admin:system:user-group:reset-query": "Reset",
                    "admin:system:user-group:view-detail": "Details",
                    "admin:account:profile:default-access": "Basic access",
                    "admin:system:user-group:view-members": "View members",
                    "admin:system:user-group:remove-members": "Remove",
                    "admin:system:dictionary:delete-selected": "Delete Selected",
                    "admin:system:user-group:delete-selected": "Delete Selected",
                    "admin:system:developer:compare-online": "Compare with online",
                    "admin:system:developer:refresh": "Refresh online catalog",
                    "admin:system:developer:publish": "Publish catalog",
                    "admin:system:action:create-group": "Create permission group",
                    "admin:system:action:reorder-groups": "Reorder permission groups",
                    "admin:system:action:delete-group": "Delete permission group"
                },
                "resources": {
                    "admin:audit": "Audit",
                    "admin:system": "System Management",
                    "admin:default": "Default permissions",
                    "admin:dashboard": "Dashboard",
                    "admin:msg:inbox": "Inbox",
                    "admin:system:msg": "Message",
                    "admin:audit:login": "Login audit",
                    "admin:system:role": "Role",
                    "admin:system:user": "User",
                    "admin:system:action": "行为",
                    "admin:account:profile": "Profile",
                    "admin:audit:operation": "Operation audit",
                    "admin:default:session": "Session and account APIs",
                    "admin:system:developer": "Developer",
                    "admin:system:dictionary": "Dictionary",
                    "admin:system:user-group": "User group",
                    "admin:dashboard:overview": "Overview",
                    "admin:dashboard:workspace": "Workspace"
                },
                "operations": {
                    "read": "View",
                    "create": "Create",
                    "delete": "Delete",
                    "update": "Edit"
                },
                "preferences": {
                    "antd": {
                        "title": "Business Preferences",
                        "fields": {
                            "tenantMode": {
                                "label": "Tenant Mode",
                                "options": {
                                    "multi": {
                                        "label": "Multi Tenant"
                                    },
                                    "single": {
                                        "label": "Single Tenant"
                                    }
                                }
                            },
                            "reportTitle": {
                                "label": "Default Report Title",
                                "placeholder": "Please enter the default report title"
                            },
                            "defaultTableSize": {
                                "label": "Default Page Size"
                            },
                            "enableFormFullscreen": {
                                "tip": "Subprojects can read this extension configuration in their own business scenarios.",
                                "label": "Enable Form Fullscreen Mode"
                            }
                        },
                        "tabLabel": "Antd Extension"
                    },
                    "mode": "Mode",
                    "wide": "Wide",
                    "other": "Other",
                    "plain": "Plain",
                    "theme": {
                        "dark": "Dark",
                        "light": "Light",
                        "title": "Theme",
                        "radius": "Radius",
                        "builtin": {
                            "gray": "Gray",
                            "pink": "Pink",
                            "rose": "Rose",
                            "zinc": "Zinc",
                            "green": "Green",
                            "slate": "Slate",
                            "title": "Built-in",
                            "custom": "Custom",
                            "orange": "Orange",
                            "violet": "Violet",
                            "yellow": "Yellow",
                            "default": "Default",
                            "neutral": "Neutral",
                            "skyBlue": "Sky Blue",
                            "deepBlue": "Deep Blue",
                            "deepGreen": "Deep Green"
                        },
                        "fontSize": "Font Size",
                        "grayMode": "Gray Mode",
                        "weakMode": "Weak Mode",
                        "darkHeader": "Semi Dark Header",
                        "darkSidebar": "Semi Dark Sidebar",
                        "fontSizeTip": "Adjust global font size with real-time preview",
                        "darkSidebarSub": "Semi Dark Sidebar Sub",
                        "darkSidebarTip": "It can be enabled when the theme is light, and the layout is neither ''Horizontal'' nor ''Full Content''.",
                        "darkSidebarSubTip": "It can be enabled when the theme is light, the semi dark sidebar is enabled, and the layout uses ''Two-Column'' menu mode."
                    },
                    "title": "Preferences",
                    "footer": {
                        "fixed": "Fixed at Bottom",
                        "title": "Footer",
                        "visible": "Show Footer"
                    },
                    "header": {
                        "title": "Header",
                        "visible": "Show Header",
                        "modeAuto": "Auto hide & Show",
                        "menuAlign": "Menu Align",
                        "modeFixed": "Fixed",
                        "modeStatic": "Static",
                        "menuAlignEnd": "End",
                        "menuAlignStart": "Start",
                        "modeAutoScroll": "Scroll to Hide & Show",
                        "menuAlignCenter": "Center"
                    },
                    "layout": "Layout",
                    "normal": "Normal",
                    "tabbar": {
                        "icon": "Show Tabbar Icon",
                        "title": "Tabbar",
                        "enable": "Enable Tab Bar",
                        "persist": "Persist Tabs",
                        "maxCount": "Max Count of Tabs",
                        "showMore": "Show More Button",
                        "draggable": "Enable Draggable Sort",
                        "styleType": {
                            "card": "Card",
                            "brisk": "Brisk",
                            "plain": "Plain",
                            "title": "Tabs Style",
                            "chrome": "Chrome"
                        },
                        "wheelable": "Support Mouse Wheel",
                        "contextMenu": {
                            "pin": "Pin",
                            "close": "Close",
                            "unpin": "Unpin",
                            "reload": "Reload",
                            "closeAll": "Close All Tabs",
                            "maximize": "Maximize",
                            "closeLeft": "Close Left Tabs",
                            "closeOther": "Close Other Tabs",
                            "closeRight": "Close Right Tabs",
                            "openInNewWindow": "Open in New Window",
                            "restoreMaximize": "Restore"
                        },
                        "maxCountTip": "When the number of tabs exceeds the maximum,\nthe oldest tab will be closed.\n Set to 0 to disable count checking.",
                        "showMaximize": "Show Maximize Button",
                        "visitHistory": "Visit History",
                        "wheelableTip": "When enabled, the Tabbar area responds to vertical scrolling events of the scroll wheel.",
                        "visitHistoryTip": "When enabled, the tab bar records tab visit history. \nClosing the current tab will automatically select the last opened tab.",
                        "middleClickClose": "Close Tab when Mouse Middle Button Click"
                    },
                    "widget": {
                        "title": "Widget",
                        "header": "Header",
                        "hidden": "Hidden",
                        "refresh": "Enable Refresh",
                        "timezone": "Enable Timezone",
                        "fullscreen": "Enable Fullscreen",
                        "lockScreen": "Enable Lock Screen",
                        "themeToggle": "Enable Theme Toggle",
                        "globalSearch": "Global Search",
                        "notification": "Enable Notification",
                        "userDropdown": "User Dropdown",
                        "sidebarToggle": "Enable Sidebar Toggle",
                        "languageToggle": "Language Toggle",
                        "refreshPosition": "Refresh Position",
                        "timezonePosition": "Timezone Position",
                        "fullscreenPosition": "Fullscreen Position",
                        "lockScreenPosition": "Lock Screen Button Position",
                        "themeTogglePosition": "Theme Toggle Position",
                        "globalSearchPosition": "Global Search Position",
                        "logoutButtonPosition": "Logout Button Position",
                        "notificationPosition": "Notification Position",
                        "languageTogglePosition": "Language Toggle Position"
                    },
                    "compact": "Fixed",
                    "content": "Content",
                    "general": "General",
                    "rounded": "Rounded",
                    "sidebar": {
                        "title": "Sidebar",
                        "width": "Width",
                        "buttons": "Show Buttons",
                        "visible": "Show Sidebar",
                        "collapsed": "Collpase Menu",
                        "draggable": "Drag Sidebar Menu",
                        "buttonFixed": "Fixed",
                        "expandOnHover": "Expand On Hover",
                        "buttonCollapsed": "Collapsed",
                        "expandOnHoverTip": "When the mouse hovers over menu, \n `Enabled` to expand children menus \n `Disabled` to expand whole sidebar.",
                        "autoActivateChild": "Auto Activate SubMenu",
                        "collapsedShowTitle": "Show Menu Title",
                        "autoActivateChildTip": "`Enabled` to automatically activate the submenu while click menu."
                    },
                    "language": "Language",
                    "position": {
                        "auto": "Auto",
                        "fixed": "Fixed",
                        "title": "Preferences Postion",
                        "header": "Header",
                        "userDropdown": "User Dropdown"
                    },
                    "resetTip": "Data has changed, click to reset",
                    "subtitle": "Customize Preferences & Preview in Real Time",
                    "timezone": "Timezone",
                    "vertical": "Vertical",
                    "animation": {
                        "title": "Animation",
                        "loading": "Page Loading",
                        "progress": "Page Progress",
                        "transition": "Page Transition"
                    },
                    "copyright": {
                        "icp": "ICP License Number",
                        "date": "Date",
                        "title": "Copyright",
                        "enable": "Enable Copyright",
                        "icpLink": "ICP Site Link",
                        "companyName": "Company Name",
                        "companySiteLink": "Company Site Link"
                    },
                    "mixedMenu": "Mixed Menu",
                    "twoColumn": "Two Column",
                    "watermark": "Watermark",
                    "appearance": "Appearance",
                    "breadcrumb": {
                        "home": "Show Home Button",
                        "icon": "Show Breadcrumb Icon",
                        "style": "Breadcrumb Style",
                        "title": "Breadcrumb",
                        "enable": "Enable Breadcrumb",
                        "background": "background",
                        "hideOnlyOne": "Hidden when only one"
                    },
                    "horizontal": "Horizontal",
                    "resetTitle": "Reset Preferences",
                    "fullContent": "Full Content",
                    "verticalTip": "Side vertical menu mode",
                    "checkUpdates": "Periodic update check",
                    "dynamicTitle": "Dynamic Title",
                    "followSystem": "Follow System",
                    "mixedMenuTip": "Vertical & Horizontal Menu Co-exists",
                    "resetSuccess": "Preferences reset successfully",
                    "shortcutKeys": {
                        "title": "Shortcut Keys",
                        "escape": "Close Current Window",
                        "global": "Global",
                        "logout": "Logout",
                        "search": "Global Search",
                        "preferences": "Preferences"
                    },
                    "twoColumnTip": "Vertical Two Column Menu Mode",
                    "horizontalTip": "Horizontal menu mode, all menus displayed at the top",
                    "clearAndLogout": "Clear Cache & Logout",
                    "fullContentTip": "Only display content body, hide all menus",
                    "navigationMenu": {
                        "split": "Navigation Menu Separation",
                        "style": "Navigation Menu Style",
                        "title": "Navigation Menu",
                        "splitTip": "When enabled, the sidebar displays the top bar''s submenu",
                        "accordion": "Sidebar Accordion Menu"
                    },
                    "copyPreferences": "Copy Preferences",
                    "headerTwoColumn": "Header Two Column",
                    "headerSidebarNav": "Header Vertical",
                    "watermarkContent": "Please input Watermark content",
                    "headerTwoColumnTip": "Header Navigation & Sidebar Two Column co-exists",
                    "headerSidebarNavTip": "Header Full Width, Sidebar Navigation Mode",
                    "enableCopyPreferences": "Show copy preferences button",
                    "copyPreferencesSuccess": "Copy successful, please override in `src/preferences.ts` under app",
                    "copyPreferencesSuccessTitle": "Copy successful",
                    "enableStickyPreferencesNavigationBar": "Enable sticky preferences navigation bar",
                    "disableStickyPreferencesNavigationBar": "Disable sticky preferences navigation bar"
                },
                "system-cards": {
                    "close": "Close",
                    "empty": "No matching records",
                    "items": "{count} items",
                    "retry": "Retry",
                    "types": {
                        "null": "Null",
                        "array": "Array",
                        "number": "Number",
                        "object": "Object",
                        "string": "Text",
                        "boolean": "Boolean",
                        "undefined": "Empty"
                    },
                    "fields": "{count} fields",
                    "scalar": "Single value",
                    "select": "Select {name}",
                    "details": "View details",
                    "loading": "Loading records",
                    "rawValue": "JSON value",
                    "lastLogin": "Last login",
                    "loadError": "Could not load the record",
                    "moreCount": "+{count}",
                    "noMembers": "No members assigned",
                    "rawRecord": "Complete record (JSON)",
                    "backToList": "Back to list",
                    "characters": "{count} characters",
                    "loginCount": "Completed logins",
                    "noDescription": "No description",
                    "noPermissions": "No permissions assigned",
                    "recordDetails": "Record details",
                    "unresolvedHint": "These stored codes no longer match existing permissions. Edit the record to remove or replace them.",
                    "failedLoginCount": "Failed login attempts",
                    "permissionGroups": "Permission groups",
                    "associatedPermissions": "Associated permissions",
                    "unresolvedPermissions": "Unresolved permission codes"
                },
                "action-editor": {
                    "none": "None",
                    "page": "Functional resource",
                    "scope": "Scope",
                    "config": "Permission mappings",
                    "effect": "Permission effects",
                    "noEntry": "Does not grant page access.",
                    "features": "Included features",
                    "generated": "Generated automatically",
                    "noMatches": "No matching resources",
                    "operation": "Allowed operation",
                    "technical": "Technical information",
                    "noFeatures": "This operation has no button features",
                    "operations": {
                        "read": "View",
                        "create": "Create",
                        "delete": "Delete",
                        "update": "Edit"
                    },
                    "invalidWord": "Select a valid resource and operation",
                    "noResources": "No resources configured for this scope",
                    "catalogError": "Unable to load the permission catalog",
                    "editExisting": "Edit existing",
                    "requiresRead": "This operation does not grant page entry. Assign view permission separately.",
                    "alreadyExists": "This resource operation already has a permission. Edit the existing permission.",
                    "invalidConfig": "Invalid resource catalog or feature configuration",
                    "catalogLoading": "Loading permission catalog",
                    "selectResource": "Select a functional resource first",
                    "frontendButtons": "Frontend buttons",
                    "searchResources": "Search resource name or identifier",
                    "selectedResource": "Selected resource",
                    "effectPlaceholder": "Select a resource and operation to preview access.",
                    "specialPermission": "This is a system capability. Its mappings are maintained in the catalog.",
                    "noSelectedFeatures": "No features selected. No buttons or associated endpoints are granted."
                },
                "system-editor": {
                    "save": "Save changes",
                    "undo": "Undo",
                    "basic": "Basic information",
                    "rules": "Access rules",
                    "access": "Permissions",
                    "cancel": "Cancel",
                    "account": "Account details",
                    "addItem": "Add {entity}",
                    "content": "Dictionary content",
                    "discard": "Discard changes",
                    "members": "Members",
                    "unsaved": "Unsaved",
                    "continue": "Keep editing",
                    "delivery": "Delivery settings",
                    "loadMore": "Load more",
                    "required": "Fields marked * are required",
                    "noMatches": "No matching records",
                    "saveError": "Unable to save. Please try again.",
                    "cancelEdit": "Cancel editing",
                    "recordInfo": "Record information",
                    "removeItem": "Remove {name}",
                    "addSelected": "Add selected ({count})",
                    "detailTitle": "{entity} details",
                    "removedItem": "Removed {name}",
                    "alreadyAdded": "Already added",
                    "associations": {
                        "user": "member",
                        "action": "permission",
                        "recipient": "recipient"
                    },
                    "discardTitle": "Discard unsaved changes?",
                    "searchMember": "Search username or code",
                    "selectedCount": "{count} selected",
                    "chooseExisting": "Select existing {entity}",
                    "messageContent": "Message content",
                    "searchRecipient": "Search username",
                    "searchPermission": "Search name, word or code",
                    "searchAssociation": "Search {entity}",
                    "highRiskPermission": "High-risk permission"
                },
                "authentication": {
                    "qq": "QQ",
                    "code": "Security code",
                    "agree": "I agree to",
                    "email": "Email",
                    "terms": "Terms",
                    "gitHub": "GitHub",
                    "google": "Google",
                    "layout": {
                        "center": "Align Center",
                        "alignLeft": "Align Left",
                        "alignRight": "Align Right"
                    },
                    "mobile": "Mobile",
                    "signUp": "Sign Up",
                    "weChat": "WeChat",
                    "codeTip": "Security code required {0} characters",
                    "qqLogin": "QQ Login",
                    "agreeTip": "Please agree to the Privacy Policy and Terms",
                    "emailTip": "Please enter email",
                    "pageDesc": "Efficient, versatile frontend template",
                    "password": "Password",
                    "sendCode": "Get Security code",
                    "sendText": "Resend in {0}s",
                    "username": "Username",
                    "goToLogin": "Login instead",
                    "mobileTip": "Please enter mobile number",
                    "pageTitle": "Plug-and-play Admin system",
                    "accountTip": "Don''t have an account?",
                    "githubLogin": "Github Login",
                    "googleLogin": "Google Login",
                    "mobileLogin": "Mobile Login",
                    "passwordTip": "Please enter password",
                    "qrcodeLogin": "QR Code Login",
                    "usernameTip": "Please enter username",
                    "wechatLogin": "Wechat Login",
                    "welcomeBack": "Welcome Back",
                    "codeSubtitle": "Enter your phone number to start managing your project",
                    "loginSuccess": "Login Successful",
                    "qrcodePrompt": "Click ''Confirm'' after scanning to complete login",
                    "createAccount": "Create Account",
                    "dingdingLogin": "Dingding Login",
                    "loginSubtitle": "Enter your account details to manage your projects",
                    "privacyPolicy": "Privacy-policy",
                    "selectAccount": "Quick Select Account",
                    "sendResetLink": "Send Reset Link",
                    "accountHistory": "Recent accounts",
                    "mobileErrortip": "The phone number format is incorrect",
                    "qrcodeSubtitle": "Scan the QR code with your phone to login",
                    "signUpSubtitle": "Make managing your applications simple and fun",
                    "confirmPassword": "Confirm Password",
                    "createAnAccount": "Create an Account",
                    "loginAgainTitle": "Please Log In Again",
                    "thirdPartyLogin": "Or continue with",
                    "loginSuccessDesc": "Welcome Back",
                    "passwordErrorTip": "Password is incorrect",
                    "verifyRequiredTip": "Please complete the verification first",
                    "alreadyHaveAccount": "Already have an account?",
                    "confirmPasswordTip": "The passwords do not match",
                    "emailValidErrorTip": "The email format you entered is incorrect",
                    "loginAgainSubTitle": "Your login session has expired. Please log in again to continue.",
                    "clearAccountHistory": "Clear account history",
                    "removeAccountHistory": "Remove account {account} from history"
                },
                "profile-center": {
                    "login": "Logins",
                    "account": "Account information",
                    "viewAll": "View all",
                    "overview": "Personal overview",
                    "security": "Account security",
                    "accountId": "Account ID",
                    "openInbox": "Open inbox",
                    "operation": "Operations",
                    "viewAudit": "View audit records",
                    "noActivity": "No recent activity",
                    "accountCode": "Account code",
                    "activityType": "Activity type",
                    "loginFailure": "Failed login",
                    "loginSuccess": "Successful login",
                    "activityRange": "Last 30 days · Latest {count} records",
                    "allPermissions": "All permissions",
                    "passwordNotice": "Changing your password signs you out on all devices, including this one.",
                    "recentActivity": "Recent activity",
                    "permissionScope": "Permissions",
                    "noActivityAccess": "Audit access not granted",
                    "accessibleFeatures": "Accessible features",
                    "assignedPermissions": "Assigned permissions"
                },
                "system-members": {
                    "count": "{count} members",
                    "remove": "Remove",
                    "changes": "Pending: add {added}, remove {removed}",
                    "viewAll": "View all {count} members"
                },
                "system-insights": {
                    "all": "All",
                    "cards": "Card view",
                    "empty": "No matching records",
                    "retry": "Retry",
                    "roles": "Roles",
                    "table": "Table view",
                    "groups": "User groups",
                    "matrix": "Scope matrix",
                    "period": "Statistics period",
                    "details": "Details",
                    "clearDay": "Clear date filter",
                    "lastDays": "Last {count} days",
                    "newUsers": "New users",
                    "noAccess": "View permission required",
                    "overview": "Management overview",
                    "selected": "{count} selected",
                    "allGroups": "All permissions",
                    "dayFilter": "Created on: {date}",
                    "emptyHint": "Adjust your search or reset the filters.",
                    "lastLogin": "Last login",
                    "loadError": "Unable to load data. Please retry.",
                    "noSummary": "No statistics available",
                    "ungrouped": "Ungrouped",
                    "userTotal": "Total users",
                    "userTrend": "New users and recent logins",
                    "clearGroup": "Clear group filter",
                    "configured": "Configured",
                    "loginCount": "Login count",
                    "newUserDay": "{date}: {count} new accounts",
                    "actionTotal": "Permissions",
                    "emptyGroups": "{count} groups have no members",
                    "groupFilter": "Business group: {name}",
                    "memberships": "Memberships",
                    "selectGroup": "Filter permission group: {name}",
                    "filterStatus": "Filter {status} users",
                    "groupMembers": "{name}: {count} members",
                    "noAccessHint": "Contact an administrator to request access.",
                    "roleOverview": "Role configuration",
                    "accountStatus": "Account status",
                    "filterEnabled": "Filter {status} dictionaries",
                    "groupOverview": "Membership overview",
                    "neverLoggedIn": "Never signed in",
                    "notConfigured": "Not configured",
                    "overviewError": "Statistics are unavailable. You can still use the list.",
                    "selectAllPage": "Select current page",
                    "uniqueMembers": "Unique members",
                    "businessGroups": "Business groups",
                    "clearRoleGroup": "Clear permission group filter: {name}",
                    "loginDayFilter": "Last login on: {date}",
                    "membersByGroup": "Members by user group",
                    "menuConfigured": "With menu paths",
                    "recentLoginDay": "{date}: {count} users last signed in",
                    "withoutMembers": "No members",
                    "roleGroupFilter": "Permission group: {name}",
                    "withPermissions": "With permissions",
                    "buttonConfigured": "With button permissions",
                    "permissionFilter": "Permission assignment: {status}",
                    "recentLoginUsers": "Users with recent logins",
                    "roleDistribution": "Permission group distribution",
                    "resourceConfigured": "With resource rules",
                    "withoutPermissions": "Without permissions",
                    "clearPermissionFilter": "Clear permission assignment filter"
                }
            }
        },
        "zh-CN": {
            "backend": {
                "ROLE_IDS": "请提供 1 至 100 个有效的角色 ID",
                "USER_IDS": "请提供 1 至 100 个有效的用户 ID",
                "ACTION_IDS": "请提供 1 至 100 个有效的权限 ID",
                "AUTH_LOCKED": "用户已被锁定",
                "MSG_INVALID": "消息字段无效：标题最多 200 字，正文最多 20000 字，过期时间须晚于当前时间",
                "AUTH_INVALID": "登录参数无效",
                "ROLE_INVALID": "角色参数无效",
                "USER_INVALID": "用户参数无效",
                "HTTP_INTERNAL": "服务器内部错误，请稍后重试",
                "MSG_NOT_FOUND": "消息不存在或不可见",
                "ROLE_CONFLICT": "角色标识已存在",
                "USERGROUP_IDS": "请提供 1 至 100 个有效的用户组 ID",
                "USER_CONFLICT": "用户名已存在",
                "ACTION_INVALID": "权限参数无效",
                "DICTIONARY_IDS": "请提供 1 至 100 个有效的数据字典 ID",
                "HTTP_FORBIDDEN": "没有权限执行此操作",
                "HTTP_NOT_FOUND": "请求的资源不存在",
                "ROLE_NOT_FOUND": "角色不存在",
                "USER_NOT_FOUND": "用户不存在",
                "ACTION_CONFLICT": "权限编码或标识已存在",
                "AUDIT_NOT_FOUND": "审计记录不存在。",
                "ACTION_NOT_FOUND": "权限不存在",
                "HTTP_BAD_REQUEST": "请求参数无效",
                "AUTH_LOGIN_FAILED": "用户名或密码错误",
                "AUTH_UNAUTHORIZED": "登录状态无效或已过期，请重新登录",
                "HTTP_INVALID_BODY": "请求内容格式无效",
                "HTTP_INVALID_PAGE": "页码 p 必须为单个 32 位整数",
                "HTTP_UNAUTHORIZED": "登录状态无效或已过期，请重新登录",
                "USERGROUP_INVALID": "用户组参数无效",
                "AUDIT_EXPORT_LIMIT": "超过 10,000 条记录，请缩小筛选范围后导出。",
                "AUTH_SAME_PASSWORD": "新密码不能与当前密码相同",
                "DICTIONARY_INVALID": "数据字典参数无效",
                "HTTP_INVALID_QUERY": "查询参数格式无效",
                "USERGROUP_CONFLICT": "用户组标识已存在",
                "DICTIONARY_CONFLICT": "数据字典键已存在",
                "USERGROUP_NOT_FOUND": "用户组不存在",
                "USER_ROLE_NOT_FOUND": "用户关联的角色不存在",
                "AUDIT_FILTER_INVALID": "审计筛选条件或时间范围无效（最长 90 天）。",
                "AUTH_USERNAME_EXISTS": "用户名已存在",
                "DICTIONARY_NOT_FOUND": "数据字典不存在",
                "HTTP_CAPTCHA_EXPIRED": "验证码已过期，请重新验证",
                "HTTP_GATEWAY_TIMEOUT": "请求超时，请稍后重试",
                "HTTP_INVALID_ROLE_ID": "角色 ID 无效",
                "HTTP_INVALID_USER_ID": "用户 ID 无效",
                "AUTH_PENDING_APPROVAL": "用户注册正在等待审核",
                "MSG_INVALID_RECIPIENT": "定向消息须选择 1–1000 个有效用户；全员消息不能指定收件人",
                "ROLE_ACTION_NOT_FOUND": "角色关联的权限不存在",
                "USER_ACTION_NOT_FOUND": "用户关联的权限不存在",
                "AUTH_SESSION_NOT_FOUND": "登录会话不存在或已过期",
                "HTTP_DUPLICATE_REQUEST": "该请求标识已使用，请勿重复提交",
                "HTTP_INVALID_ACTION_ID": "权限 ID 无效",
                "HTTP_INVALID_PAGE_SIZE": "每页条数 s 必须为单个 32 位整数",
                "AUTH_INCORRECT_PASSWORD": "当前密码错误",
                "AUTH_INFO_SCOPE_INVALID": "权限作用域无效",
                "AUTH_INVALID_CREDENTIAL": "登录凭证无效或已过期，请重试",
                "HTTP_METHOD_NOT_ALLOWED": "不支持此请求方法",
                "SYSTEM_FEATURE_DISABLED": "系统已禁用此功能",
                "AUTH_CHALLENGE_NOT_FOUND": "凭证挑战已过期，请重试",
                "AUTH_INFO_SCOPE_REQUIRED": "请指定权限作用域",
                "HTTP_INVALID_USER_STATUS": "状态必须为 0 至 2 的值，多个值用逗号分隔",
                "HTTP_SERVICE_UNAVAILABLE": "服务暂不可用，请稍后重试",
                "USERGROUP_USER_NOT_FOUND": "用户组成员不存在",
                "AUTH_INVALID_REGISTRATION": "注册参数无效",
                "HTTP_INVALID_DICTIONARY_ID": "数据字典 ID 无效",
                "HTTP_INVALID_KEYWORD_QUERY": "keyword 参数只能提供一个值",
                "HTTP_INVALID_USER_GROUP_ID": "用户组 ID 无效",
                "USERGROUP_ACTION_NOT_FOUND": "用户组关联的权限不存在",
                "AUTH_INVALID_PASSWORD_RESET": "请输入新密码",
                "AUTH_PASSWORD_CHANGE_LOCKED": "自助修改密码已锁定，请联系管理员",
                "AUTH_POINT_CAPTCHA_REQUIRED": "验证码无效，请重新验证",
                "DICTIONARY_VERSION_CONFLICT": "目录已被修改，请对比线上最新版本后再提交",
                "HTTP_INVALID_CATEGORY_VALUE": "类别必须为 0 或 1",
                "HTTP_INVALID_PUBLIC_PURPOSE": "purpose 必须为 login 或 register",
                "HTTP_INVALID_USERNAME_QUERY": "username 参数只能提供一个值",
                "AUTH_INVALID_PASSWORD_CHANGE": "修改密码的参数无效",
                "AUTH_PASSWORD_RESET_REQUIRED": "请先重置密码再继续使用",
                "AUTH_POINT_CAPTCHA_NOT_FOUND": "验证码已过期，请重新验证",
                "AUTH_SLIDER_CAPTCHA_REQUIRED": "请重新完成滑块验证",
                "HTTP_INVALID_CATEGORY_FORMAT": "类别必须为单个 16 位整数",
                "HTTP_INVALID_IDEMPOTENCY_KEY": "幂等性标识无效",
                "HTTP_INVALID_PASSWORD_PURPOSE": "purpose 必须为 password_change 或 password_reset",
                "AUTH_PERMISSION_ADMIN_REQUIRED": "仅拥有当前作用域全部权限的管理员可以修改权限定义或账户凭证",
                "AUTH_PERMISSION_GRANT_REQUIRED": "需要当前作用域的全部权限或权限分配权限",
                "HTTP_INVALID_DICTIONARY_ENABLED": "enabled 必须为单个布尔值",
                "AUTH_INVALID_PASSWORD_CREDENTIAL": "修改密码的凭证无效或已过期，请重试",
                "AUTH_PASSWORD_RESET_NOT_REQUIRED": "当前账号无需首次重置密码",
                "AUTH_INVALID_REGISTRATION_CREDENTIAL": "注册凭证无效或已过期，请重试"
            },
            "frontend": {
                "ui": {
                    "crop": {
                        "title": "图片裁剪",
                        "cancel": "取消裁剪",
                        "confirm": "裁剪",
                        "errorTip": "裁剪错误",
                        "titleTip": "裁剪比例 {0}"
                    },
                    "tiptap": {
                        "upload": {
                            "fileTooLarge": "文件大小超出限制",
                            "uploadFailed": "上传失败",
                            "onlySingleImage": "仅支持单张图片上传，已选取第一张",
                            "fileTypeNotAllowed": "不支持的文件类型"
                        },
                        "prompts": {
                            "link": "请输入链接地址",
                            "image": "请输入图片地址"
                        },
                        "toolbar": {
                            "bold": "加粗",
                            "code": "行内代码",
                            "link": "链接",
                            "redo": "重做",
                            "undo": "撤销",
                            "clear": "清除",
                            "image": "图片",
                            "italic": "斜体",
                            "strike": "删除线",
                            "unlink": "移除链接",
                            "heading": "标题",
                            "preview": "预览",
                            "heading1": "标题1",
                            "heading2": "标题2",
                            "heading3": "标题3",
                            "heading4": "标题4",
                            "imageUrl": "图片URL",
                            "alignLeft": "左对齐",
                            "codeBlock": "代码块",
                            "paragraph": "正文",
                            "textColor": "文字颜色",
                            "underline": "下划线",
                            "alignRight": "右对齐",
                            "blockquote": "引用",
                            "bulletList": "无序列表",
                            "alignCenter": "居中",
                            "imageUpload": "从本地上传",
                            "orderedList": "有序列表",
                            "highlightColor": "高亮颜色"
                        },
                        "placeholder": "请输入内容..."
                    },
                    "captcha": {
                        "alt": "支持img标签src属性值",
                        "title": "请完成安全验证",
                        "confirm": "确认",
                        "clickInOrder": "请依次点击",
                        "pointAriaLabel": "点击点",
                        "confirmAriaLabel": "确认选择",
                        "refreshAriaLabel": "刷新验证码",
                        "sliderDefaultText": "请按住滑块拖动",
                        "sliderSuccessText": "验证通过",
                        "sliderRotateFailTip": "验证失败",
                        "sliderRotateDefaultTip": "点击图片可刷新",
                        "sliderRotateSuccessTip": "验证成功，耗时{0}秒",
                        "sliderTranslateFailTip": "验证失败",
                        "sliderTranslateDefaultTip": "点击图片可刷新",
                        "sliderTranslateSuccessTip": "验证成功，耗时{0}秒"
                    },
                    "widgets": {
                        "qa": "问题 & 帮助",
                        "search": {
                            "close": "关闭",
                            "title": "搜索",
                            "recent": "搜索历史",
                            "select": "选择",
                            "history": "历史记录",
                            "results": "搜索结果",
                            "navigate": "导航",
                            "noRecent": "没有搜索历史",
                            "noResults": "未找到搜索结果",
                            "removeHistory": "从历史记录移除 {value}",
                            "searchNavigate": "搜索导航菜单"
                        },
                        "setting": "设置",
                        "viewAll": "查看所有消息",
                        "document": "文档",
                        "timezone": {
                            "setSuccess": "时区设置成功",
                            "setTimezone": "设置时区"
                        },
                        "logoutTip": "是否退出登录？",
                        "lockScreen": {
                            "entry": "进入系统",
                            "title": "锁定屏幕",
                            "unlock": "点击解锁",
                            "password": "密码",
                            "backToLogin": "返回登录",
                            "placeholder": "请输入锁屏密码",
                            "screenButton": "锁定",
                            "errorPasswordTip": "密码错误，请重新输入"
                        },
                        "markAllAsRead": "全部标记为已读",
                        "notifications": "通知",
                        "checkUpdatesTitle": "新版本可用",
                        "clearNotifications": "清空",
                        "checkUpdatesDescription": "点击刷新以获取最新版本"
                    },
                    "fallback": {
                        "http": {
                            "notFound": "未找到, 请求的资源不存在。",
                            "forbidden": "禁止访问, 您没有权限访问此资源。",
                            "badRequest": "请求错误。请检查您的输入并重试。",
                            "networkError": "网络异常，请检查您的网络连接后重试。",
                            "unauthorized": "登录认证过期，请重新登录后继续。",
                            "requestTimeout": "请求超时，请稍后再试。",
                            "internalServerError": "内部服务器错误，请稍后再试。"
                        },
                        "offline": "离线页面",
                        "forbidden": "哎呀！访问被拒绝",
                        "comingSoon": "即将推出",
                        "offlineError": "哎呀！网络错误",
                        "pageNotFound": "哎呀！未找到页面",
                        "forbiddenDesc": "抱歉，您没有权限访问此页面。",
                        "internalError": "哎呀！出错了",
                        "offlineErrorDesc": "抱歉，无法连接到互联网，请检查您的网络连接并重试。",
                        "pageNotFoundDesc": "抱歉，我们无法找到您要找的页面。",
                        "internalErrorDesc": "抱歉，服务器遇到错误。"
                    },
                    "formRules": {
                        "length": "{0}长度必须为{1}个字符",
                        "required": "请输入{0}",
                        "maxLength": "{0}最多{1}个字符",
                        "minLength": "{0}至少{1}个字符",
                        "sizeLimit": "文件大小不能超过 {0}MB",
                        "startWith": "{0}必须以 {1} 开头",
                        "invalidURL": "请输入有效的链接",
                        "alreadyExists": "{0} `{1}` 已存在",
                        "previewWarning": "无法打开文件，没有可用的URL或预览地址",
                        "selectRequired": "请选择{0}"
                    },
                    "iconPicker": {
                        "search": "搜索图标...",
                        "placeholder": "选择一个图标"
                    },
                    "jsonViewer": {
                        "copy": "复制",
                        "copied": "已复制"
                    },
                    "actionTitle": {
                        "edit": "修改{0}",
                        "view": "查看{0}",
                        "create": "新增{0}",
                        "delete": "删除{0}"
                    },
                    "placeholder": {
                        "input": "请输入",
                        "select": "请选择",
                        "upload": "点击上传"
                    },
                    "actionMessage": {
                        "deleting": "正在删除 {0} ...",
                        "deleteConfirm": "确定删除 {0} 吗？",
                        "deleteSuccess": "{0} 删除成功",
                        "operationFailed": "操作失败",
                        "operationSuccess": "操作成功"
                    }
                },
                "app": {
                    "errors": {
                        "failed": "请求失败，请稍后重试。",
                        "dataLoad": "数据加载失败，请重试。",
                        "serverLost": "服务器迷路啦~",
                        "statisticsLoad": "统计信息加载失败，请重试。"
                    },
                    "captcha": {
                        "start": "点击开始验证",
                        "failed": "顺序错误，请重试",
                        "passed": "验证通过",
                        "checking": "正在校验…",
                        "progress": "验证中…",
                        "additional": "安全验证",
                        "sliderFailed": "验证失败，请稍慢地拖到最右侧后重试",
                        "sliderIncomplete": "请将滑块拖到最右侧"
                    },
                    "journey": {
                        "title": "欢迎归航",
                        "entering": "登录成功，正在进入…",
                        "description": "进入 Go Cinch，继续今天的工作"
                    },
                    "register": {
                        "success": "账号创建成功，请等待管理员审核后登录。",
                        "usernameExists": "用户名已存在"
                    },
                    "validation": {
                        "password": "请输入密码",
                        "username": "请输入用户名",
                        "verification": "请先完成验证",
                        "currentPassword": "请输入当前密码",
                        "passwordRequired": "请输入密码",
                        "credentialsRequired": "请输入用户名和密码",
                        "passwordChangeRequired": "请输入当前密码和新密码"
                    },
                    "resetPassword": {
                        "title": "重置密码",
                        "logout": "退出登录",
                        "submit": "重置并继续",
                        "success": "密码已重置",
                        "description": "首次登录或管理员重置密码后，请设置一个与当前密码不同的新密码，再开始使用。"
                    }
                },
                "msg": {
                    "all": "全员",
                    "hint": "查看通知，管理你的已读和删除状态。",
                    "next": "下一页",
                    "page": "第 {page} 页，共 {total} 条",
                    "read": "已读",
                    "send": "发送消息",
                    "sent": "发送成功",
                    "type": "消息类型",
                    "view": "查看",
                    "close": "关闭",
                    "count": "未读 {count} 条",
                    "empty": "暂无消息",
                    "error": "加载失败，请重试",
                    "inbox": "我的消息",
                    "plain": "简洁",
                    "retry": "重试",
                    "saved": "操作成功",
                    "scope": "发送范围",
                    "style": "列表样式",
                    "title": "标题",
                    "cancel": "取消",
                    "delete": "删除",
                    "detail": "消息详情",
                    "expiry": "过期时间",
                    "manage": "消息",
                    "normal": "标准",
                    "notice": "通知公告",
                    "select": "选择",
                    "sender": "发送人 ID",
                    "system": "系统消息",
                    "unread": "未读",
                    "actions": "操作",
                    "columns": "显示字段",
                    "compact": "紧凑",
                    "confirm": "确认",
                    "content": "正文",
                    "density": "密度",
                    "expired": "已过期",
                    "feedAll": "全部",
                    "feedEnd": "已显示全部消息",
                    "history": "历史消息",
                    "keyword": "关键词",
                    "loading": "正在加载消息…",
                    "preview": "消息预览",
                    "readAll": "全部已读",
                    "refresh": "刷新",
                    "striped": "斑马纹",
                    "unsaved": "放弃未发送的消息？",
                    "allTypes": "全部类型",
                    "markRead": "标记已读",
                    "noExpiry": "永不过期",
                    "pageSize": "每页条数",
                    "previous": "上一页",
                    "targeted": "指定用户",
                    "allScopes": "全部范围",
                    "allStatus": "全部状态",
                    "feedOrder": "按发布时间倒序",
                    "feedShown": "已展示 {count} / {total} 条",
                    "published": "发布时间",
                    "selectAll": "全选",
                    "splitView": "分栏视图",
                    "tableView": "表格视图",
                    "feedCenter": "消息中心",
                    "feedExpand": "展开全文",
                    "fullscreen": "全屏",
                    "manageHint": "发布全员或定向通知，查看发送记录。",
                    "recipients": "收件人",
                    "searchUser": "搜索用户名",
                    "titleError": "请输入标题，最多 200 字",
                    "detailError": "消息详情加载失败，请重试。",
                    "expiryError": "请选择有效的未来时间",
                    "feedFilters": "筛选消息",
                    "feedNoMatch": "没有找到匹配的消息",
                    "feedUpdates": "消息已更新，点击刷新",
                    "contentError": "请输入正文，最多 20000 字",
                    "deleteGlobal": "全局删除",
                    "feedCollapse": "收起全文",
                    "feedLoadMore": "向下滚动或点击加载更多",
                    "feedNoUnread": "暂无未读消息",
                    "noPermission": "没有查看发送记录的权限",
                    "noRecipients": "暂无收件人",
                    "recipientIDs": "收件人 ID",
                    "summaryError": "消息统计加载失败，仍可查看发布记录。",
                    "activeFilters": "已选筛选条件",
                    "broadcastHint": "仅覆盖消息发布时已经注册的用户。",
                    "deleteConfirm": "从我的收件箱删除这条消息？",
                    "messageFilter": "消息：{title}",
                    "selectPreview": "选择一条消息查看内容",
                    "viewRecipient": "查看用户 {username}",
                    "deleteSelected": "批量删除",
                    "publishedCount": "已发布消息",
                    "recipientCount": "共 {count} 人",
                    "recipientError": "请选择 1–1000 位收件人",
                    "searchMessages": "搜索消息标题或正文",
                    "feedLoadingMore": "正在加载更早消息…",
                    "latestPublished": "最新发布",
                    "selectedMessage": "指定消息",
                    "deletedRecipient": "用户已删除",
                    "feedSelectLoaded": "选择已加载消息",
                    "focusUnavailable": "该消息已不可用，或不符合当前筛选条件。",
                    "publishedRecords": "发布记录",
                    "typeDistribution": "通知公告 {notice} 条，系统消息 {system} 条",
                    "scopeDistribution": "全员消息 {all} 条，指定用户消息 {targeted} 条",
                    "clearMessageFilter": "清除消息筛选",
                    "clearPreviewConfirm": "从我的收件箱删除当前通知列表中的 {count} 条消息？",
                    "deleteGlobalConfirm": "删除后所有收件人均无法查看，且无法恢复。确认删除？",
                    "readSelectedConfirm": "将选中的消息标为已读？",
                    "deleteSelectedConfirm": "删除选中的消息？仅从你的收件箱删除。",
                    "recipientsUnavailable": "收件人信息不可用",
                    "deleteSelectedGlobalConfirm": "为所有接收者删除选中的消息？"
                },
                "page": {
                    "auth": {
                        "login": "登录",
                        "profile": "个人中心",
                        "register": "注册",
                        "codeLogin": "验证码登录",
                        "qrcodeLogin": "二维码登录"
                    },
                    "profile": {
                        "tabs": {
                            "changePassword": "修改密码",
                            "basicInformation": "基本信息"
                        },
                        "title": "个人中心",
                        "fields": {
                            "role": "角色",
                            "username": "用户名"
                        },
                        "password": {
                            "success": "密码修改成功，所有设备均需重新登录",
                            "newPassword": "新密码",
                            "oldPassword": "当前密码",
                            "confirmPassword": "确认密码",
                            "passwordMismatch": "两次输入的密码不一致",
                            "newPasswordPlaceholder": "请输入新密码",
                            "oldPasswordPlaceholder": "请输入当前密码",
                            "confirmPasswordPlaceholder": "请再次输入新密码"
                        }
                    },
                    "dashboard": {
                        "title": "概览",
                        "overview": "概览",
                        "workspace": "工作台",
                        "overviewPage": {
                            "tabs": {
                                "monthlyVisits": "月访问量",
                                "trafficTrends": "流量趋势"
                            },
                            "cards": {
                                "visitCount": "访问数量",
                                "businessMix": "商业占比",
                                "visitSource": "访问来源"
                            },
                            "charts": {
                                "web": "网页",
                                "month": "{month}月",
                                "other": "其它",
                                "trend": "趋势",
                                "visit": "访问",
                                "client": "客户端",
                                "direct": "直接访问",
                                "mobile": "移动端",
                                "remote": "远程",
                                "tablet": "平板端",
                                "thirdParty": "第三方",
                                "businessMix": "商业占比",
                                "outsourcing": "外包",
                                "visitSource": "访问来源",
                                "affiliateAds": "联盟广告",
                                "searchEngine": "搜索引擎",
                                "customization": "定制",
                                "emailMarketing": "邮件营销",
                                "technicalSupport": "技术支持"
                            },
                            "metrics": {
                                "usage": "使用量",
                                "users": "用户量",
                                "visits": "访问量",
                                "downloads": "下载量",
                                "totalUsage": "总使用量",
                                "totalUsers": "总用户量",
                                "totalVisits": "总访问量",
                                "totalDownloads": "总下载量"
                            }
                        },
                        "workspacePage": {
                            "stats": {
                                "teams": "团队",
                                "todos": "待办",
                                "projects": "项目"
                            },
                            "todos": {
                                "uiIssue": {
                                    "title": "修复 UI 显示问题",
                                    "content": "修复用户报告的页面 UI 显示问题，确保在不同浏览器中显示一致。"
                                },
                                "security": {
                                    "title": "安全检查",
                                    "content": "进行系统安全检查，确保没有安全漏洞或未授权的访问。"
                                },
                                "codeReview": {
                                    "title": "审查前端代码提交",
                                    "content": "审查最近提交到 Git 仓库的前端代码，确保代码质量和规范。"
                                },
                                "performance": {
                                    "title": "系统性能优化",
                                    "content": "检查并优化系统性能，降低 CPU 使用率。"
                                },
                                "dependencies": {
                                    "title": "更新项目依赖",
                                    "content": "更新项目中的所有 npm 依赖包，确保使用最新的兼容版本。"
                                }
                            },
                            "groups": {
                                "web": "Web 组",
                                "algorithm": "算法组",
                                "openSource": "开源组",
                                "technology": "技术组",
                                "architecture": "架构组"
                            },
                            "header": {
                                "weather": "今日晴，20℃–32℃。",
                                "greeting": "早上好，{name}，开始您一天的工作吧！"
                            },
                            "people": {
                                "evan": "艾文",
                                "jack": "杰克",
                                "chris": "克里斯",
                                "peter": "皮特",
                                "william": "威廉"
                            },
                            "trends": {
                                "dates": {
                                    "justNow": "刚刚",
                                    "oneDayAgo": "1 天前",
                                    "oneHourAgo": "1 小时前",
                                    "oneWeekAgo": "1 周前",
                                    "twoDaysAgo": "2 天前",
                                    "threeDaysAgo": "3 天前"
                                },
                                "items": {
                                    "postedUpdate": "发布了个人动态",
                                    "pushedGithub": "推送了代码到 GitHub",
                                    "publishedVite": "发表了《如何编写 Vite 插件》",
                                    "closedRunIssue": "关闭了问题《如何运行项目》",
                                    "createdProject": "在开源组创建了 Vue 项目",
                                    "followedWilliam": "关注了威廉",
                                    "publishedAdminVben": "发表了《如何使用 go cinch》",
                                    "repliedOptimization": "回复了杰克的问题《如何进行项目优化？》"
                                }
                            },
                            "projects": {
                                "vueQuote": "现在的你决定将来的你。",
                                "htmlQuote": "没有什么才能比努力更重要。",
                                "reactQuote": "健康的身体是实现目标的基石。",
                                "githubQuote": "不要等待机会，而要创造机会。",
                                "angularQuote": "热情和欲望可以突破一切难关。",
                                "javascriptQuote": "路是走出来的，而不是空想出来的。"
                            },
                            "sections": {
                                "todos": "待办事项",
                                "projects": "项目",
                                "latestActivity": "最新动态",
                                "quickNavigation": "快捷导航"
                            },
                            "navigation": {
                                "home": "首页",
                                "charts": "图表",
                                "dashboard": "仪表盘",
                                "components": "组件",
                                "accessManagement": "权限",
                                "systemManagement": "系统"
                            }
                        }
                    },
                    "notifications": {
                        "dates": {
                            "justNow": "刚刚",
                            "fixedDate": "2024-01-01",
                            "oneDayAgo": "1天前",
                            "threeHoursAgo": "3小时前"
                        },
                        "titles": {
                            "replied": "朱偏右回复了你",
                            "commented": "曲丽丽评论了你",
                            "externalLink": "跳转外部链接示例",
                            "todoReminder": "待办提醒",
                            "weeklyReports": "收到了 14 份新周报",
                            "workspaceLink": "跳转工作台示例"
                        },
                        "description": "这是通知的描述信息。"
                    }
                },
                "audit": {
                    "id": "事件 ID",
                    "ip": "来源 IP",
                    "all": "全部",
                    "p95": "P95 耗时",
                    "less": "少",
                    "menu": "审计",
                    "more": "多",
                    "path": "请求路径",
                    "peak": "高峰：{day} {hour}:00 · {count} 次",
                    "actor": "操作人",
                    "empty": "所选范围内暂无记录",
                    "login": "登录日志",
                    "reset": "重置",
                    "retry": "重试",
                    "total": "事件总量",
                    "trend": "事件趋势",
                    "client": "客户端",
                    "copied": "已复制",
                    "events": "事件明细",
                    "export": "导出 CSV",
                    "failed": "失败次数",
                    "method": "请求方法",
                    "module": "业务模块",
                    "period": "时间范围",
                    "search": "搜索",
                    "status": "结果",
                    "actions": {
                        "create": "新增{module}",
                        "remove": "删除{module}",
                        "update": "更新{module}"
                    },
                    "average": "平均耗时",
                    "dayView": "天",
                    "details": "详情",
                    "failure": "失败",
                    "heatmap": "登录活跃热力图",
                    "insight": "时段洞察",
                    "latency": "响应耗时",
                    "modules": {
                        "msg": "消息",
                        "auth": "认证",
                        "role": "角色",
                        "user": "用户",
                        "action": "权限",
                        "session": "会话",
                        "dictionary": "字典",
                        "user-group": "用户组"
                    },
                    "noTrace": "未记录",
                    "period1": "最近 24 小时",
                    "period7": "最近 7 天",
                    "request": "请求信息",
                    "success": "成功",
                    "user_id": "操作人 ID",
                    "business": "业务操作",
                    "continue": "继续追查",
                    "copyPath": "复制请求路径",
                    "heatCell": "{day} {hour}:00，登录 {count} 次，失败 {failed} 次",
                    "heatPeak": "高峰：{time} · {count} 次",
                    "heatView": "热力",
                    "hourView": "小时",
                    "noAccess": "你没有查看这些审计记录的权限。",
                    "noParams": "未记录参数",
                    "outcomes": "结果分布",
                    "period30": "最近 30 天",
                    "samePath": "相同请求",
                    "trace_id": "追踪 ID",
                    "username": "账号",
                    "weekday0": "周一",
                    "weekday1": "周二",
                    "weekday2": "周三",
                    "weekday3": "周四",
                    "weekday4": "周五",
                    "weekday5": "周六",
                    "weekday6": "周日",
                    "anonymous": "未知账号",
                    "copyError": "复制失败，请选中文本后复制。",
                    "copyTrace": "复制追踪 ID",
                    "errorInfo": "错误信息",
                    "loadError": "审计记录加载失败，请重试。",
                    "localTime": "当前所选时区",
                    "noSources": "暂无来源 IP",
                    "openEvent": "查看记录 {id}",
                    "operation": "操作日志",
                    "sameActor": "同一操作人",
                    "timeFirst": "时间优先",
                    "trendView": "趋势",
                    "uniqueIPs": "来源 IP",
                    "writeOnly": "业务写操作",
                    "allModules": "全部模块",
                    "allPeriods": "全部时段",
                    "allRecords": "最新记录",
                    "byDuration": "按耗时",
                    "error_code": "错误代码",
                    "httpStatus": "HTTP 状态",
                    "loginCount": "登录次数",
                    "noFailures": "该范围内暂无失败",
                    "paramsHint": "敏感值已隐藏，文件内容以 file 代替。",
                    "resourceId": "对象 ID",
                    "restoreAll": "恢复全部",
                    "searchHint": "账号、IP、路径或追踪 ID",
                    "topEntries": "前 {count} 名",
                    "user_agent": "客户端信息",
                    "closeDetail": "关闭详情",
                    "duration_ms": "耗时",
                    "eventDetail": "事件详情",
                    "exportError": "导出失败，请重试或将范围缩小至 10,000 条以内。",
                    "hasFailures": "含失败事件",
                    "occurred_at": "发生时间",
                    "otherClient": "其他客户端",
                    "recordCount": "{count} 条记录",
                    "recordOrder": "记录顺序",
                    "sourceFocus": "活跃来源",
                    "successRate": "成功率",
                    "uniqueUsers": "登录账号",
                    "choosePeriod": "选择时段",
                    "expandDetail": "展开详情",
                    "failureFirst": "失败优先",
                    "heatInterval": "{start} — {end}，{count} 次登录，{failed} 次失败",
                    "heatmapLevel": "热力图粒度",
                    "loginAttempt": "登录尝试",
                    "loginRecords": "登录记录",
                    "resourceType": "对象类型",
                    "selectRecord": "选择一条记录",
                    "filterApplied": "已带入账号筛选：{value}",
                    "loginActivity": "登录活跃时段",
                    "loginAttempts": "登录尝试",
                    "operationType": "操作类型",
                    "requestFailed": "请求未成功完成。",
                    "requestParams": "请求参数",
                    "failedAccounts": "失败账号",
                    "operationCount": "操作次数",
                    "operationTypes": {
                        "read": "标为已读",
                        "create": "创建",
                        "delete": "删除",
                        "logout": "退出登录",
                        "update": "修改",
                        "read_all": "全部已读",
                        "register": "注册",
                        "reset_password": "重置密码",
                        "change_password": "修改密码",
                        "delete_personal": "从收件箱删除"
                    },
                    "responseParams": "响应参数",
                    "selectedPeriod": "已选时段",
                    "activityLoading": "正在加载活跃时段",
                    "failureDominant": "失败偏多",
                    "hourUnavailable": "{hour}:00，此时段暂无可查看的数据",
                    "operationRecords": "操作记录",
                    "repeatedFailures": "重复失败账号",
                    "timeDistribution": "时间分布",
                    "operationActivity": "操作活动",
                    "moduleDistribution": "模块操作分布",
                    "noRepeatedFailures": "暂无重复失败账号"
                },
                "demos": {
                    "antd": "Ant Design Vue",
                    "vben": {
                        "about": "关于",
                        "antdv": "Ant Design Vue 版本",
                        "title": "项目",
                        "tdesign": "TDesign Vue 版本",
                        "document": "文档",
                        "naive-ui": "Naive UI 版本",
                        "antdv-next": "Antdv Next 版本",
                        "element-plus": "Element Plus 版本"
                    },
                    "title": "演示"
                },
                "common": {
                    "no": "否",
                    "yes": "是",
                    "back": "返回",
                    "edit": "修改",
                    "more": "更多",
                    "login": "登录",
                    "query": "查询",
                    "reset": "重置",
                    "cancel": "取消",
                    "create": "新增",
                    "delete": "删除",
                    "detail": "详情",
                    "logout": "退出登录",
                    "noData": "暂无数据",
                    "prompt": "提示",
                    "search": "搜索",
                    "confirm": "确认",
                    "enabled": "已启用",
                    "notShow": "不显示",
                    "refresh": "刷新",
                    "disabled": "已禁用",
                    "backToHome": "返回首页",
                    "hideSearchPanel": "隐藏搜索面板",
                    "showSearchPanel": "显示搜索面板"
                },
                "groups": {
                    "admin:system:msg": "站内信",
                    "admin:audit:login": "登录日志",
                    "admin:system:role": "角色",
                    "admin:system:user": "用户",
                    "admin:system:action": "行为",
                    "admin:system:general": "通用",
                    "admin:audit:operation": "操作日志",
                    "admin:system:dictionary": "字典",
                    "admin:system:user-group": "用户组",
                    "admin:dashboard:overview": "仪表盘",
                    "admin:dashboard:workspace": "工作台",
                    "admin:system:permission-admin": "访问管理",
                    "admin:system:developer": "开发者"
                },
                "scopes": {
                    "h5": "移动网页",
                    "app": "应用",
                    "mini": "小程序",
                    "admin": "管理后台"
                },
                "system": {
                    "menu": {
                        "roles": "角色",
                        "users": "用户",
                        "groups": "用户组",
                        "system": "系统",
                        "actions": "权限",
                        "dictionary": "数据字典"
                    },
                    "role": {
                        "title": "角色管理"
                    },
                    "user": {
                        "lock": "锁定",
                        "title": "用户管理",
                        "entity": "用户",
                        "reason": "原因",
                        "reject": "拒绝",
                        "review": "注册审核",
                        "unlock": "解锁",
                        "approve": "通过",
                        "decision": "审核结果",
                        "lockType": "锁定方式",
                        "lockLabel": "锁定：",
                        "lockTitle": "锁定用户“{name}”",
                        "lockUntil": "至指定时间",
                        "permanent": "永久",
                        "lockedUntil": "锁定截止时间",
                        "untilPrefix": "截至",
                        "keepPassword": "留空则保留当前密码",
                        "unlockPassword": "解锁密码修改",
                        "rejectionPlaceholder": "请输入注册驳回原因"
                    },
                    "hints": {
                        "createGroup": "创建“{name}”"
                    },
                    "table": {
                        "loose": "宽松",
                        "style": "表格样式",
                        "total": "共 {count} 条",
                        "sticky": "固定表头",
                        "columns": "列设置",
                        "compact": "紧凑",
                        "default": "默认",
                        "density": "表格密度",
                        "striped": "斑马纹",
                        "bordered": "边框",
                        "fullscreen": "全屏",
                        "exitFullscreen": "退出全屏",
                        "visibleColumns": "显示列"
                    },
                    "action": {
                        "title": "权限管理",
                        "entity": "权限"
                    },
                    "common": {
                        "edit": "编辑",
                        "less": "收起",
                        "more": "展开",
                        "enter": "请输入{field}",
                        "reset": "重置",
                        "create": "新增",
                        "delete": "删除",
                        "search": "搜索",
                        "select": "请选择{field}",
                        "actions": "操作",
                        "refresh": "刷新",
                        "unknown": "未知",
                        "editTitle": "编辑{entity}",
                        "selectAll": "全选",
                        "selection": "选择列",
                        "createTitle": "新增{entity}",
                        "deleteSelected": "批量删除"
                    },
                    "fields": {
                        "id": "ID",
                        "code": "编码",
                        "name": "名称",
                        "role": "角色",
                        "word": "标识",
                        "group": "分组",
                        "status": "状态",
                        "enabled": "启用",
                        "members": "成员",
                        "category": "类别",
                        "menuPath": "菜单路径",
                        "metadata": "元数据",
                        "password": "密码",
                        "resource": "资源",
                        "userCode": "用户编码",
                        "username": "用户名",
                        "createdAt": "创建时间",
                        "menuPaths": "菜单路径",
                        "updatedAt": "更新时间",
                        "actionCode": "权限编码",
                        "description": "说明",
                        "newPassword": "新密码",
                        "permissions": "权限",
                        "dictionaryKey": "字典键",
                        "resourceRules": "资源规则",
                        "dictionaryValue": "字典值（JSON）",
                        "buttonPermission": "按钮权限"
                    },
                    "status": {
                        "active": "正常",
                        "locked": "已锁定",
                        "pending": "待审核"
                    },
                    "confirm": {
                        "delete": "确认删除“{name}”？",
                        "unlock": "确认解锁“{name}”？",
                        "deleteSelected": "确认删除选中的记录？",
                        "unlockPassword": "确认解锁“{name}”的自助修改密码功能？",
                        "deleteDescription": "将删除选中的 {count} 条记录。"
                    },
                    "enabled": {
                        "no": "已停用",
                        "yes": "已启用"
                    },
                    "category": {
                        "jwt": "JWT",
                        "permission": "权限"
                    },
                    "messages": {
                        "locked": "用户锁定成功",
                        "created": "创建成功",
                        "deleted": "删除成功",
                        "updated": "修改成功",
                        "approved": "注册审核已通过",
                        "rejected": "注册申请已驳回",
                        "unlocked": "用户解锁成功",
                        "noChanges": "没有需要保存的修改",
                        "passwordUnlocked": "自助修改密码已解锁"
                    },
                    "userGroup": {
                        "title": "用户组管理",
                        "entity": "用户组"
                    },
                    "dictionary": {
                        "title": "数据字典管理",
                        "entity": "数据字典"
                    },
                    "validation": {
                        "json": "请输入有效的 JSON",
                        "required": "请输入{field}",
                        "futureLock": "请选择未来的锁定截止时间",
                        "dictionaryKey": "请使用大写字母、数字，并以下划线分隔各段",
                        "rejectionRequired": "请输入驳回原因"
                    },
                    "suggestions": {
                        "role": "按角色名称或标识搜索",
                        "user": "按用户名或编码搜索",
                        "action": "按权限名称或标识搜索",
                        "backend": "查询结果",
                        "history": "历史记录",
                        "searching": "搜索中…",
                        "actionGroup": "搜索或创建权限分组",
                        "removeHistory": "从历史记录中移除“{value}”"
                    }
                },
                "actions": {
                    "admin:read:system:msg": "查看站内信",
                    "admin:read:audit:login": "查看登录日志",
                    "admin:read:system:role": "查看角色",
                    "admin:read:system:user": "查看用户",
                    "admin:create:system:msg": "发送站内信",
                    "admin:delete:system:msg": "删除站内信",
                    "admin:create:system:role": "新增角色",
                    "admin:create:system:user": "新增用户",
                    "admin:delete:system:role": "删除角色",
                    "admin:delete:system:user": "删除用户",
                    "admin:read:system:action": "查看权限",
                    "admin:update:system:role": "编辑角色",
                    "admin:update:system:user": "编辑用户",
                    "admin:create:system:action": "新增权限",
                    "admin:delete:system:action": "删除权限",
                    "admin:read:audit:operation": "查看操作日志",
                    "admin:update:system:action": "编辑权限",
                    "admin:read:system:dictionary": "查看数据字典",
                    "admin:read:system:user-group": "查看用户组",
                    "admin:read:dashboard:overview": "查看仪表盘",
                    "admin:create:system:dictionary": "新增数据字典",
                    "admin:create:system:user-group": "新增用户组",
                    "admin:delete:system:dictionary": "删除数据字典",
                    "admin:delete:system:user-group": "删除用户组",
                    "admin:read:dashboard:workspace": "查看工作台",
                    "admin:update:system:dictionary": "编辑数据字典",
                    "admin:update:system:user-group": "编辑用户组",
                    "admin:all:system:general": "全部权限",
                    "admin:default:system:general": "默认权限",
                    "admin:grant:system:permission-admin": "权限分配",
                    "admin:update:system:developer": "维护开发者配置"
                },
                "buttons": {
                    "admin:btn:msg:inbox:query": "搜索",
                    "admin:btn:system:msg:send": "发送消息",
                    "admin:btn:msg:inbox:delete": "删除",
                    "admin:btn:system:msg:query": "搜索",
                    "admin:btn:system:role:edit": "编辑",
                    "admin:btn:system:user:edit": "编辑",
                    "admin:btn:system:user:lock": "锁定",
                    "admin:btn:audit:login:query": "搜索",
                    "admin:btn:msg:inbox:refresh": "刷新",
                    "admin:btn:system:msg:delete": "删除",
                    "admin:btn:system:role:query": "搜索",
                    "admin:btn:system:user:query": "搜索",
                    "admin:btn:msg:inbox:read-all": "全部已读",
                    "admin:btn:system:action:edit": "编辑",
                    "admin:btn:system:msg:refresh": "刷新",
                    "admin:btn:system:role:create": "新增",
                    "admin:btn:system:role:delete": "删除",
                    "admin:btn:system:user:create": "新增",
                    "admin:btn:system:user:delete": "删除",
                    "admin:btn:system:user:unlock": "解锁",
                    "admin:btn:audit:login:refresh": "刷新",
                    "admin:btn:msg:inbox:mark-read": "标记已读",
                    "admin:btn:system:action:query": "搜索",
                    "admin:btn:system:role:refresh": "刷新",
                    "admin:btn:system:user:approve": "通过",
                    "admin:btn:system:user:refresh": "刷新",
                    "admin:btn:system:action:create": "新增",
                    "admin:btn:system:action:delete": "删除",
                    "admin:btn:audit:operation:query": "搜索",
                    "admin:btn:msg:inbox:view-detail": "详情",
                    "admin:btn:system:action:refresh": "刷新",
                    "admin:btn:system:dictionary:edit": "编辑",
                    "admin:btn:system:msg:reset-query": "重置",
                    "admin:btn:system:msg:view-detail": "详情",
                    "admin:btn:system:user-group:edit": "编辑",
                    "admin:btn:audit:login:reset-query": "重置",
                    "admin:btn:audit:login:view-detail": "详情",
                    "admin:btn:audit:operation:refresh": "刷新",
                    "admin:btn:system:dictionary:query": "搜索",
                    "admin:btn:system:role:reset-query": "重置",
                    "admin:btn:system:role:view-detail": "详情",
                    "admin:btn:system:user-group:query": "搜索",
                    "admin:btn:system:user:reset-query": "重置",
                    "admin:btn:system:user:view-detail": "详情",
                    "admin:btn:system:dictionary:create": "新增",
                    "admin:btn:system:dictionary:delete": "删除",
                    "admin:btn:system:user-group:create": "新增",
                    "admin:btn:system:user-group:delete": "删除",
                    "admin:btn:audit:login:export-report": "导出 CSV",
                    "admin:btn:msg:inbox:delete-selected": "批量删除",
                    "admin:btn:system:action:reset-query": "重置",
                    "admin:btn:system:action:view-detail": "详情",
                    "admin:btn:system:dictionary:refresh": "刷新",
                    "admin:btn:system:user-group:refresh": "刷新",
                    "admin:btn:system:msg:delete-selected": "批量删除",
                    "admin:btn:audit:operation:reset-query": "重置",
                    "admin:btn:audit:operation:view-detail": "详情",
                    "admin:btn:system:role:delete-selected": "批量删除",
                    "admin:btn:system:user:delete-selected": "批量删除",
                    "admin:btn:system:user:unlock-password": "解锁密码修改",
                    "admin:btn:msg:inbox:mark-selected-read": "标记已读",
                    "admin:btn:audit:operation:export-report": "导出 CSV",
                    "admin:btn:system:action:delete-selected": "批量删除",
                    "admin:btn:system:dictionary:reset-query": "重置",
                    "admin:btn:system:dictionary:view-detail": "详情",
                    "admin:btn:system:user-group:add-members": "添加成员",
                    "admin:btn:system:user-group:reset-query": "重置",
                    "admin:btn:system:user-group:view-detail": "详情",
                    "admin:btn:system:user-group:view-members": "查看成员",
                    "admin:btn:account:profile:change-password": "修改密码",
                    "admin:btn:system:user-group:remove-members": "移除",
                    "admin:btn:system:dictionary:delete-selected": "批量删除",
                    "admin:btn:system:user-group:delete-selected": "批量删除",
                    "admin:btn:system:developer:compare-online": "对比线上",
                    "admin:btn:system:developer:refresh": "刷新线上配置",
                    "admin:btn:system:developer:publish": "提交到线上",
                    "admin:btn:system:action:create-group": "新增权限分组",
                    "admin:btn:system:action:reorder-groups": "调整分组顺序",
                    "admin:btn:system:action:delete-group": "删除权限分组"
                },
                "profile": {
                    "updatePassword": "更新密码",
                    "updateBasicProfile": "更新基本信息"
                },
                "features": {
                    "admin:system:msg:send": "发送消息",
                    "admin:system:msg:query": "搜索",
                    "admin:system:role:edit": "编辑",
                    "admin:system:user:edit": "编辑",
                    "admin:system:user:lock": "锁定",
                    "admin:audit:login:query": "搜索",
                    "admin:system:msg:delete": "删除",
                    "admin:system:role:query": "搜索",
                    "admin:system:user:query": "搜索",
                    "admin:system:action:edit": "编辑",
                    "admin:system:msg:refresh": "刷新",
                    "admin:system:role:create": "新增",
                    "admin:system:role:delete": "删除",
                    "admin:system:user:create": "新增",
                    "admin:system:user:delete": "删除",
                    "admin:system:user:unlock": "解锁",
                    "admin:audit:login:refresh": "刷新",
                    "admin:system:action:query": "搜索",
                    "admin:system:role:refresh": "刷新",
                    "admin:system:user:approve": "通过",
                    "admin:system:user:refresh": "刷新",
                    "admin:system:action:create": "新增",
                    "admin:system:action:delete": "删除",
                    "admin:audit:operation:query": "搜索",
                    "admin:system:action:refresh": "刷新",
                    "admin:default:session:access": "会话与账号操作",
                    "admin:system:dictionary:edit": "编辑",
                    "admin:system:msg:reset-query": "重置",
                    "admin:system:msg:view-detail": "详情",
                    "admin:system:user-group:edit": "编辑",
                    "admin:audit:login:reset-query": "重置",
                    "admin:audit:login:view-detail": "详情",
                    "admin:audit:operation:refresh": "刷新",
                    "admin:system:dictionary:query": "搜索",
                    "admin:system:role:reset-query": "重置",
                    "admin:system:role:view-detail": "详情",
                    "admin:system:user-group:query": "搜索",
                    "admin:system:user:reset-query": "重置",
                    "admin:system:user:view-detail": "详情",
                    "admin:msg:inbox:default-access": "基础访问",
                    "admin:system:dictionary:create": "新增",
                    "admin:system:dictionary:delete": "删除",
                    "admin:system:user-group:create": "新增",
                    "admin:system:user-group:delete": "删除",
                    "admin:audit:login:export-report": "导出 CSV",
                    "admin:system:action:reset-query": "重置",
                    "admin:system:action:view-detail": "详情",
                    "admin:system:dictionary:refresh": "刷新",
                    "admin:system:user-group:refresh": "刷新",
                    "admin:system:msg:delete-selected": "批量删除",
                    "admin:audit:operation:reset-query": "重置",
                    "admin:audit:operation:view-detail": "详情",
                    "admin:system:role:delete-selected": "批量删除",
                    "admin:system:user:delete-selected": "批量删除",
                    "admin:system:user:unlock-password": "解锁密码修改",
                    "admin:audit:operation:export-report": "导出 CSV",
                    "admin:system:action:delete-selected": "批量删除",
                    "admin:system:dictionary:reset-query": "重置",
                    "admin:system:dictionary:view-detail": "详情",
                    "admin:system:user-group:add-members": "添加成员",
                    "admin:system:user-group:reset-query": "重置",
                    "admin:system:user-group:view-detail": "详情",
                    "admin:account:profile:default-access": "基础访问",
                    "admin:system:user-group:view-members": "查看成员",
                    "admin:system:user-group:remove-members": "移除",
                    "admin:system:dictionary:delete-selected": "批量删除",
                    "admin:system:user-group:delete-selected": "批量删除",
                    "admin:system:developer:compare-online": "对比线上",
                    "admin:system:developer:refresh": "刷新线上配置",
                    "admin:system:developer:publish": "提交到线上",
                    "admin:system:action:create-group": "新增权限分组",
                    "admin:system:action:reorder-groups": "调整分组顺序",
                    "admin:system:action:delete-group": "删除权限分组"
                },
                "resources": {
                    "admin:audit": "审计",
                    "admin:system": "系统",
                    "admin:default": "默认权限",
                    "admin:dashboard": "仪表盘",
                    "admin:msg:inbox": "消息中心",
                    "admin:system:msg": "站内信",
                    "admin:audit:login": "登录日志",
                    "admin:system:role": "角色",
                    "admin:system:user": "用户",
                    "admin:system:action": "行为",
                    "admin:account:profile": "个人中心",
                    "admin:audit:operation": "操作日志",
                    "admin:default:session": "会话与账号接口",
                    "admin:system:developer": "开发者",
                    "admin:system:dictionary": "字典",
                    "admin:system:user-group": "用户组",
                    "admin:dashboard:overview": "概览",
                    "admin:dashboard:workspace": "工作台"
                },
                "operations": {
                    "read": "查看",
                    "create": "新增",
                    "delete": "删除",
                    "update": "编辑"
                },
                "preferences": {
                    "antd": {
                        "title": "业务偏好",
                        "fields": {
                            "tenantMode": {
                                "label": "租户模式",
                                "options": {
                                    "multi": {
                                        "label": "多租户"
                                    },
                                    "single": {
                                        "label": "单租户"
                                    }
                                }
                            },
                            "reportTitle": {
                                "label": "报表默认标题",
                                "placeholder": "请输入报表默认标题"
                            },
                            "defaultTableSize": {
                                "label": "默认分页条数"
                            },
                            "enableFormFullscreen": {
                                "tip": "子项目可在自己的业务场景中读取这个拓展配置。",
                                "label": "启用表单全屏模式"
                            }
                        },
                        "tabLabel": "Antd 拓展"
                    },
                    "mode": "模式",
                    "wide": "流式",
                    "other": "其它",
                    "plain": "朴素",
                    "theme": {
                        "dark": "深色",
                        "light": "浅色",
                        "title": "主题",
                        "radius": "圆角",
                        "builtin": {
                            "gray": "中灰色",
                            "pink": "樱花粉",
                            "rose": "玫瑰红",
                            "zinc": "锌色灰",
                            "green": "浅绿色",
                            "slate": "石板灰",
                            "title": "内置主题",
                            "custom": "自定义",
                            "orange": "橙黄色",
                            "violet": "紫罗兰",
                            "yellow": "柠檬黄",
                            "default": "默认",
                            "neutral": "中性色",
                            "skyBlue": "天蓝色",
                            "deepBlue": "深蓝色",
                            "deepGreen": "深绿色"
                        },
                        "fontSize": "字体大小",
                        "grayMode": "灰色模式",
                        "weakMode": "色弱模式",
                        "darkHeader": "深色顶栏",
                        "darkSidebar": "深色侧边栏",
                        "fontSizeTip": "调整全局字体大小，实时预览效果",
                        "darkSidebarSub": "深色侧边栏子栏",
                        "darkSidebarTip": "当主题为浅色，布局不为水平菜单或不为内容全屏时可开启",
                        "darkSidebarSubTip": "当主题为浅色，开启深色侧边栏且布局使用双列菜单模式时可开启"
                    },
                    "title": "偏好设置",
                    "footer": {
                        "fixed": "固定在底部",
                        "title": "底栏",
                        "visible": "显示底栏"
                    },
                    "header": {
                        "title": "顶栏",
                        "visible": "显示顶栏",
                        "modeAuto": "自动隐藏和显示",
                        "menuAlign": "菜单位置",
                        "modeFixed": "固定",
                        "modeStatic": "静止",
                        "menuAlignEnd": "右侧",
                        "menuAlignStart": "左侧",
                        "modeAutoScroll": "滚动隐藏和显示",
                        "menuAlignCenter": "居中"
                    },
                    "layout": "布局",
                    "normal": "常规",
                    "tabbar": {
                        "icon": "显示标签栏图标",
                        "title": "标签栏",
                        "enable": "启用标签栏",
                        "persist": "持久化标签页",
                        "maxCount": "最大标签数",
                        "showMore": "显示更多按钮",
                        "draggable": "启动拖拽排序",
                        "styleType": {
                            "card": "卡片",
                            "brisk": "轻快",
                            "plain": "朴素",
                            "title": "标签页风格",
                            "chrome": "谷歌"
                        },
                        "wheelable": "启用纵向滚轮响应",
                        "contextMenu": {
                            "pin": "固定",
                            "close": "关闭",
                            "unpin": "取消固定",
                            "reload": "重新加载",
                            "closeAll": "关闭全部标签页",
                            "maximize": "最大化",
                            "closeLeft": "关闭左侧标签页",
                            "closeOther": "关闭其它标签页",
                            "closeRight": "关闭右侧标签页",
                            "openInNewWindow": "在新窗口打开",
                            "restoreMaximize": "还原"
                        },
                        "maxCountTip": "每次打开新的标签时如果超过最大标签数，\n会自动关闭一个最先打开的标签\n设置为 0 则不限制",
                        "showMaximize": "显示最大化按钮",
                        "visitHistory": "访问历史记录",
                        "wheelableTip": "开启后，标签栏区域可以响应滚轮的纵向滚动事件。\n关闭时，只能响应系统的横向滚动事件（需要按下Shift再滚动滚轮）",
                        "visitHistoryTip": "开启后，标签栏会记录标签访问历史\n关闭当前标签，会自动选中上一个打开的标签",
                        "middleClickClose": "点击鼠标中键关闭标签页"
                    },
                    "widget": {
                        "title": "小部件",
                        "header": "顶栏",
                        "hidden": "已隐藏",
                        "refresh": "刷新",
                        "timezone": "时区",
                        "fullscreen": "全屏",
                        "lockScreen": "锁屏",
                        "themeToggle": "主题切换",
                        "globalSearch": "全局搜索",
                        "notification": "通知",
                        "userDropdown": "用户下拉窗",
                        "sidebarToggle": "启用侧边栏切换",
                        "languageToggle": "语言切换",
                        "refreshPosition": "刷新按钮位置",
                        "timezonePosition": "时区按钮位置",
                        "fullscreenPosition": "全屏按钮位置",
                        "lockScreenPosition": "锁屏按钮位置",
                        "themeTogglePosition": "主题切换位置",
                        "globalSearchPosition": "全局搜索位置",
                        "logoutButtonPosition": "退出按钮位置",
                        "notificationPosition": "通知位置",
                        "languageTogglePosition": "语言切换位置"
                    },
                    "compact": "定宽",
                    "content": "内容",
                    "general": "通用",
                    "rounded": "圆润",
                    "sidebar": {
                        "title": "侧边栏",
                        "width": "宽度",
                        "buttons": "显示按钮",
                        "visible": "显示侧边栏",
                        "collapsed": "折叠菜单",
                        "draggable": "侧边栏菜单拖拽",
                        "buttonFixed": "固定按钮",
                        "expandOnHover": "鼠标悬停展开",
                        "buttonCollapsed": "折叠按钮",
                        "expandOnHoverTip": "鼠标在折叠区域悬浮时，`启用`则展开当前子菜单，`禁用`则展开整个侧边栏",
                        "autoActivateChild": "自动激活子菜单",
                        "collapsedShowTitle": "折叠显示菜单名",
                        "autoActivateChildTip": "点击顶层菜单时,自动激活第一个子菜单或者上一次激活的子菜单"
                    },
                    "language": "语言",
                    "position": {
                        "auto": "自动",
                        "fixed": "固定",
                        "title": "偏好设置位置",
                        "header": "顶栏",
                        "userDropdown": "用户下拉窗"
                    },
                    "resetTip": "数据有变化，点击可进行重置",
                    "subtitle": "自定义偏好设置 & 实时预览",
                    "timezone": "时区",
                    "vertical": "垂直",
                    "animation": {
                        "title": "动画",
                        "loading": "页面切换 Loading",
                        "progress": "页面切换进度条",
                        "transition": "页面切换动画"
                    },
                    "copyright": {
                        "icp": "ICP 备案号",
                        "date": "日期",
                        "title": "版权",
                        "enable": "启用版权",
                        "icpLink": "ICP 网站链接",
                        "companyName": "公司名",
                        "companySiteLink": "公司主页"
                    },
                    "mixedMenu": "混合垂直",
                    "twoColumn": "双列菜单",
                    "watermark": "水印",
                    "appearance": "外观",
                    "breadcrumb": {
                        "home": "显示首页按钮",
                        "icon": "显示面包屑图标",
                        "style": "面包屑风格",
                        "title": "面包屑导航",
                        "enable": "开启面包屑导航",
                        "background": "背景",
                        "hideOnlyOne": "仅有一个时隐藏"
                    },
                    "horizontal": "水平",
                    "resetTitle": "重置偏好设置",
                    "fullContent": "内容全屏",
                    "verticalTip": "侧边垂直菜单模式",
                    "checkUpdates": "定时检查更新",
                    "dynamicTitle": "动态标题",
                    "followSystem": "跟随系统",
                    "mixedMenuTip": "垂直水平菜单共存",
                    "resetSuccess": "重置偏好设置成功",
                    "shortcutKeys": {
                        "title": "快捷键",
                        "escape": "关闭当前窗口",
                        "global": "全局",
                        "logout": "退出登录",
                        "search": "全局搜索",
                        "preferences": "偏好设置"
                    },
                    "twoColumnTip": "垂直双列菜单模式",
                    "horizontalTip": "水平菜单模式，菜单全部显示在顶部",
                    "clearAndLogout": "清空缓存 & 退出登录",
                    "fullContentTip": "不显示任何菜单，只显示内容主体",
                    "navigationMenu": {
                        "split": "导航菜单分离",
                        "style": "导航菜单风格",
                        "title": "导航菜单",
                        "splitTip": "开启时，侧边栏显示顶栏对应菜单的子菜单",
                        "accordion": "侧边导航菜单手风琴模式"
                    },
                    "copyPreferences": "复制偏好设置",
                    "headerTwoColumn": "混合双列",
                    "headerSidebarNav": "侧边导航",
                    "watermarkContent": "请输入水印文案",
                    "headerTwoColumnTip": "双列、水平菜单共存模式",
                    "headerSidebarNavTip": "顶部通栏，侧边导航模式",
                    "enableCopyPreferences": "显示复制偏好设置按钮",
                    "copyPreferencesSuccess": "复制成功，请在 app 下的 `src/preferences.ts`内进行覆盖",
                    "copyPreferencesSuccessTitle": "复制成功",
                    "enableStickyPreferencesNavigationBar": "开启首选项导航栏吸顶效果",
                    "disableStickyPreferencesNavigationBar": "关闭首选项导航栏吸顶效果"
                },
                "system-cards": {
                    "close": "关闭",
                    "empty": "没有匹配的记录",
                    "items": "{count} 个条目",
                    "retry": "重试",
                    "types": {
                        "null": "空值",
                        "array": "数组",
                        "number": "数字",
                        "object": "对象",
                        "string": "文本",
                        "boolean": "布尔值",
                        "undefined": "未设置"
                    },
                    "fields": "{count} 个字段",
                    "scalar": "单个值",
                    "select": "选择 {name}",
                    "details": "查看详情",
                    "loading": "正在加载记录",
                    "rawValue": "JSON 值",
                    "lastLogin": "最近登录",
                    "loadError": "无法加载记录",
                    "moreCount": "+{count}",
                    "noMembers": "尚未添加成员",
                    "rawRecord": "完整记录（JSON）",
                    "backToList": "返回列表",
                    "characters": "{count} 个字符",
                    "loginCount": "完成登录次数",
                    "noDescription": "暂无说明",
                    "noPermissions": "尚未关联权限",
                    "recordDetails": "记录详情",
                    "unresolvedHint": "这些已保存的编码没有对应的权限记录，可通过编辑移除或重新配置。",
                    "failedLoginCount": "登录失败次数",
                    "permissionGroups": "权限分组",
                    "associatedPermissions": "关联权限",
                    "unresolvedPermissions": "已失效的权限编码"
                },
                "action-editor": {
                    "none": "无",
                    "page": "功能资源",
                    "scope": "作用域",
                    "config": "权限映射",
                    "effect": "权限效果",
                    "noEntry": "不包含页面访问权限。",
                    "features": "包含的功能",
                    "generated": "自动生成",
                    "noMatches": "没有匹配的功能资源",
                    "operation": "允许的操作",
                    "technical": "技术信息",
                    "noFeatures": "此操作没有按钮功能",
                    "operations": {
                        "read": "查看",
                        "create": "新增",
                        "delete": "删除",
                        "update": "编辑"
                    },
                    "invalidWord": "请选择有效的功能资源和操作",
                    "noResources": "此作用域尚未配置功能资源",
                    "catalogError": "权限目录加载失败",
                    "editExisting": "编辑现有权限",
                    "requiresRead": "此操作不授予页面入口，需要另行配置查看权限。",
                    "alreadyExists": "该资源的此操作已有权限，请编辑现有权限",
                    "invalidConfig": "权限目录或功能配置无效",
                    "catalogLoading": "正在加载权限目录",
                    "selectResource": "请先选择功能资源",
                    "frontendButtons": "前端按钮",
                    "searchResources": "搜索功能名称或标识",
                    "selectedResource": "已选资源",
                    "effectPlaceholder": "选择资源和操作后展示权限范围。",
                    "specialPermission": "这是系统授权能力，其映射由目录统一维护。",
                    "noSelectedFeatures": "未选择功能，不授予按钮及其关联接口。"
                },
                "system-editor": {
                    "save": "保存修改",
                    "undo": "撤销",
                    "basic": "基本信息",
                    "rules": "访问规则",
                    "access": "权限配置",
                    "cancel": "取消",
                    "account": "账号信息",
                    "addItem": "添加{entity}",
                    "content": "字典内容",
                    "discard": "放弃修改",
                    "members": "成员管理",
                    "unsaved": "未保存",
                    "continue": "继续编辑",
                    "delivery": "发送设置",
                    "loadMore": "加载更多",
                    "required": "标有 * 的字段为必填项",
                    "noMatches": "没有找到匹配的记录",
                    "saveError": "保存失败，请重试。",
                    "cancelEdit": "取消编辑",
                    "recordInfo": "记录信息",
                    "removeItem": "移除{name}",
                    "addSelected": "添加已选（{count}）",
                    "detailTitle": "{entity}详情",
                    "removedItem": "已移除「{name}」",
                    "alreadyAdded": "已添加",
                    "associations": {
                        "user": "成员",
                        "action": "权限",
                        "recipient": "接收人"
                    },
                    "discardTitle": "放弃尚未保存的修改？",
                    "searchMember": "搜索用户名或编码",
                    "selectedCount": "已选 {count} 项",
                    "chooseExisting": "选择已有{entity}",
                    "messageContent": "消息内容",
                    "searchRecipient": "搜索用户名",
                    "searchPermission": "搜索名称、标识或编码",
                    "searchAssociation": "搜索{entity}",
                    "highRiskPermission": "高风险权限"
                },
                "authentication": {
                    "qq": "QQ",
                    "code": "验证码",
                    "agree": "我同意",
                    "email": "邮箱",
                    "terms": "条款",
                    "gitHub": "GitHub",
                    "google": "Google",
                    "layout": {
                        "center": "居中",
                        "alignLeft": "居左",
                        "alignRight": "居右"
                    },
                    "mobile": "手机号码",
                    "signUp": "注册",
                    "weChat": "微信",
                    "codeTip": "请输入{0}位验证码",
                    "qqLogin": "QQ登录",
                    "agreeTip": "请同意隐私政策和条款",
                    "emailTip": "请输入邮箱",
                    "pageDesc": "工程化、高性能、跨组件库的前端模版",
                    "password": "密码",
                    "sendCode": "获取验证码",
                    "sendText": "{0}秒后重新获取",
                    "username": "账号",
                    "goToLogin": "去登录",
                    "mobileTip": "请输入手机号",
                    "pageTitle": "开箱即用的大型中后台管理系统",
                    "accountTip": "还没有账号?",
                    "githubLogin": "Github登录",
                    "googleLogin": "Google登录",
                    "mobileLogin": "手机号登录",
                    "passwordTip": "请输入密码",
                    "qrcodeLogin": "扫码登录",
                    "usernameTip": "请输入用户名",
                    "wechatLogin": "微信登录",
                    "welcomeBack": "欢迎回来",
                    "codeSubtitle": "请输入您的手机号码以开始管理您的项目",
                    "loginSuccess": "登录成功",
                    "qrcodePrompt": "扫码后点击 ''确认''，即可完成登录",
                    "createAccount": "创建账号",
                    "dingdingLogin": "钉钉登录",
                    "loginSubtitle": "请输入您的账户信息以开始管理您的项目",
                    "privacyPolicy": "隐私政策",
                    "selectAccount": "快速选择账号",
                    "sendResetLink": "发送重置链接",
                    "accountHistory": "最近登录账号",
                    "mobileErrortip": "手机号码格式错误",
                    "qrcodeSubtitle": "请用手机扫描二维码登录",
                    "signUpSubtitle": "让您的应用程序管理变得简单而有趣",
                    "confirmPassword": "确认密码",
                    "createAnAccount": "创建一个账号",
                    "loginAgainTitle": "重新登录",
                    "thirdPartyLogin": "其他登录方式",
                    "loginSuccessDesc": "欢迎回来",
                    "passwordErrorTip": "密码错误",
                    "verifyRequiredTip": "请先完成验证",
                    "alreadyHaveAccount": "已经有账号了?",
                    "confirmPasswordTip": "两次输入的密码不一致",
                    "emailValidErrorTip": "你输入的邮箱格式不正确",
                    "loginAgainSubTitle": "您的登录状态已过期，请重新登录以继续。",
                    "clearAccountHistory": "清空登录历史",
                    "removeAccountHistory": "移除历史账号 {account}"
                },
                "profile-center": {
                    "login": "登录",
                    "account": "账号信息",
                    "viewAll": "查看全部",
                    "overview": "个人概览",
                    "security": "账号安全",
                    "accountId": "账号 ID",
                    "openInbox": "打开收件箱",
                    "operation": "操作",
                    "viewAudit": "查看审计记录",
                    "noActivity": "暂无近期活动",
                    "accountCode": "账号标识",
                    "activityType": "活动类型",
                    "loginFailure": "登录失败",
                    "loginSuccess": "登录成功",
                    "activityRange": "最近 30 天 · 最新 {count} 条",
                    "allPermissions": "全部权限",
                    "passwordNotice": "修改密码后，所有已登录设备（包括当前设备）需要重新登录。",
                    "recentActivity": "最近活动",
                    "permissionScope": "权限范围",
                    "noActivityAccess": "暂无审计查看权限",
                    "accessibleFeatures": "可访问功能",
                    "assignedPermissions": "已授权功能"
                },
                "system-members": {
                    "count": "共 {count} 人",
                    "remove": "移除",
                    "changes": "待保存：新增 {added} 人，移除 {removed} 人",
                    "viewAll": "查看全部 {count} 人"
                },
                "system-insights": {
                    "all": "全部",
                    "cards": "卡片视图",
                    "empty": "没有匹配的记录",
                    "retry": "重试",
                    "roles": "角色",
                    "table": "表格视图",
                    "groups": "用户组",
                    "matrix": "范围矩阵",
                    "period": "统计时间范围",
                    "details": "详情",
                    "clearDay": "清除日期筛选",
                    "lastDays": "最近 {count} 天",
                    "newUsers": "新增用户",
                    "noAccess": "暂无查看权限",
                    "overview": "管理概览",
                    "selected": "已选 {count} 条",
                    "allGroups": "全部权限",
                    "dayFilter": "新增日期：{date}",
                    "emptyHint": "调整搜索条件，或重置筛选后重试。",
                    "lastLogin": "最近登录",
                    "loadError": "数据加载失败，请重试。",
                    "noSummary": "暂无统计数据",
                    "ungrouped": "未分组",
                    "userTotal": "用户总数",
                    "userTrend": "新增与最近登录",
                    "clearGroup": "清除分组筛选",
                    "configured": "已配置",
                    "loginCount": "登录次数",
                    "newUserDay": "{date}：新增 {count} 个账号",
                    "actionTotal": "权限总数",
                    "emptyGroups": "{count} 个用户组暂无成员",
                    "groupFilter": "业务分组：{name}",
                    "memberships": "成员关联",
                    "selectGroup": "筛选权限分组：{name}",
                    "filterStatus": "筛选{status}用户",
                    "groupMembers": "{name}：{count} 位成员",
                    "noAccessHint": "请联系管理员申请对应的查看权限。",
                    "roleOverview": "角色配置概览",
                    "accountStatus": "账号状态",
                    "filterEnabled": "筛选{status}字典",
                    "groupOverview": "协作范围",
                    "neverLoggedIn": "从未登录",
                    "notConfigured": "未配置",
                    "overviewError": "暂时无法加载统计信息，列表仍可使用。",
                    "selectAllPage": "选择当前页",
                    "uniqueMembers": "去重成员",
                    "businessGroups": "业务分组",
                    "clearRoleGroup": "清除权限分组筛选：{name}",
                    "loginDayFilter": "最近登录日期：{date}",
                    "membersByGroup": "各用户组成员数量",
                    "menuConfigured": "含菜单路径",
                    "recentLoginDay": "{date}：最近一次登录的用户 {count} 人",
                    "withoutMembers": "暂无成员",
                    "roleGroupFilter": "权限分组：{name}",
                    "withPermissions": "已分配权限",
                    "buttonConfigured": "含按钮权限",
                    "permissionFilter": "权限分配：{status}",
                    "recentLoginUsers": "最近登录用户数",
                    "roleDistribution": "权限分组分布",
                    "resourceConfigured": "含资源规则",
                    "withoutPermissions": "未分配权限",
                    "clearPermissionFilter": "清除权限分配筛选"
                }
            }
        }
    },
    "operations": [
        "read",
        "create",
        "update",
        "delete"
    ],
    "group_order": [
        "admin:system:permission-admin",
        "admin:system:general",
        "admin:dashboard:overview",
        "admin:dashboard:workspace",
        "admin:system:user",
        "admin:system:role",
        "admin:system:user-group",
        "admin:system:action",
        "admin:system:developer",
        "admin:system:dictionary",
        "admin:system:msg",
        "admin:audit:login",
        "admin:audit:operation"
    ],
    "default_locale": "zh-CN",
    "schema_version": 2,
    "resource_catalog": {
        "buttons": {
            "admin:system:msg:send": {
                "btn": [
                    "admin:btn:system:msg:send"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg",
                        "method": "POST"
                    },
                    {
                        "path": "/msg/recipient-option",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:msg:query": {
                "btn": [
                    "admin:btn:system:msg:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/sent",
                        "method": "GET"
                    },
                    {
                        "path": "/msg/sent/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:role:edit": {
                "btn": [
                    "admin:btn:system:role:edit"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:user:edit": {
                "btn": [
                    "admin:btn:system:user:edit"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:user:lock": {
                "btn": [
                    "admin:btn:system:user:lock"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:audit:login:query": {
                "btn": [
                    "admin:btn:audit:login:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/login",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/login/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/login/option",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:msg:delete": {
                "btn": [
                    "admin:btn:system:msg:delete"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/sent/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:role:query": {
                "btn": [
                    "admin:btn:system:role:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role",
                        "method": "GET"
                    },
                    {
                        "path": "/role/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user:query": {
                "btn": [
                    "admin:btn:system:user:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user",
                        "method": "GET"
                    },
                    {
                        "path": "/user/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:edit": {
                "btn": [
                    "admin:btn:system:action:edit"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:msg:refresh": {
                "btn": [
                    "admin:btn:system:msg:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/sent",
                        "method": "GET"
                    },
                    {
                        "path": "/msg/sent/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:role:create": {
                "btn": [
                    "admin:btn:system:role:create"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role",
                        "method": "POST"
                    }
                ]
            },
            "admin:system:role:delete": {
                "btn": [
                    "admin:btn:system:role:delete"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:user:create": {
                "btn": [
                    "admin:btn:system:user:create"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user",
                        "method": "POST"
                    }
                ]
            },
            "admin:system:user:delete": {
                "btn": [
                    "admin:btn:system:user:delete"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:user:unlock": {
                "btn": [
                    "admin:btn:system:user:unlock"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:audit:login:refresh": {
                "btn": [
                    "admin:btn:audit:login:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/login",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/login/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/login/option",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:query": {
                "btn": [
                    "admin:btn:system:action:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action",
                        "method": "GET"
                    },
                    {
                        "path": "/action/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/action/group",
                        "method": "GET"
                    },
                    {
                        "path": "/action/catalog",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:role:refresh": {
                "btn": [
                    "admin:btn:system:role:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role",
                        "method": "GET"
                    },
                    {
                        "path": "/role/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user:approve": {
                "btn": [
                    "admin:btn:system:user:approve"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:user:refresh": {
                "btn": [
                    "admin:btn:system:user:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user",
                        "method": "GET"
                    },
                    {
                        "path": "/user/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:create": {
                "btn": [
                    "admin:btn:system:action:create"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action",
                        "method": "POST"
                    }
                ]
            },
            "admin:system:action:delete": {
                "btn": [
                    "admin:btn:system:action:delete"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:audit:operation:query": {
                "btn": [
                    "admin:btn:audit:operation:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/operation",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/operation/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/operation/option",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:refresh": {
                "btn": [
                    "admin:btn:system:action:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action",
                        "method": "GET"
                    },
                    {
                        "path": "/action/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/action/group",
                        "method": "GET"
                    },
                    {
                        "path": "/action/catalog",
                        "method": "GET"
                    }
                ]
            },
            "admin:default:session:access": {
                "btn": [],
                "grpc": [],
                "http": [
                    {
                        "path": "/auth/info",
                        "method": "GET"
                    },
                    {
                        "path": "/auth/challenge",
                        "method": "POST"
                    },
                    {
                        "path": "/auth/reset/pwd",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:dictionary:edit": {
                "btn": [
                    "admin:btn:system:dictionary:edit"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:msg:reset-query": {
                "btn": [
                    "admin:btn:system:msg:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/sent",
                        "method": "GET"
                    },
                    {
                        "path": "/msg/sent/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:msg:view-detail": {
                "btn": [
                    "admin:btn:system:msg:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/sent/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user-group:edit": {
                "btn": [
                    "admin:btn:system:user-group:edit"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:audit:login:reset-query": {
                "btn": [
                    "admin:btn:audit:login:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/login",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/login/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/login/option",
                        "method": "GET"
                    }
                ]
            },
            "admin:audit:login:view-detail": {
                "btn": [
                    "admin:btn:audit:login:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/login/detail/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:audit:operation:refresh": {
                "btn": [
                    "admin:btn:audit:operation:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/operation",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/operation/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/operation/option",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:dictionary:query": {
                "btn": [
                    "admin:btn:system:dictionary:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary",
                        "method": "GET"
                    },
                    {
                        "path": "/dictionary/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:role:reset-query": {
                "btn": [
                    "admin:btn:system:role:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role",
                        "method": "GET"
                    },
                    {
                        "path": "/role/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:role:view-detail": {
                "btn": [
                    "admin:btn:system:role:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user-group:query": {
                "btn": [
                    "admin:btn:system:user-group:query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group",
                        "method": "GET"
                    },
                    {
                        "path": "/user-group/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user:reset-query": {
                "btn": [
                    "admin:btn:system:user:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user",
                        "method": "GET"
                    },
                    {
                        "path": "/user/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user:view-detail": {
                "btn": [
                    "admin:btn:system:user:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:msg:inbox:default-access": {
                "btn": [
                    "admin:btn:msg:inbox:query",
                    "admin:btn:msg:inbox:refresh",
                    "admin:btn:msg:inbox:view-detail",
                    "admin:btn:msg:inbox:mark-read",
                    "admin:btn:msg:inbox:mark-selected-read",
                    "admin:btn:msg:inbox:read-all",
                    "admin:btn:msg:inbox:delete",
                    "admin:btn:msg:inbox:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/inbox",
                        "method": "GET"
                    },
                    {
                        "path": "/msg/inbox/{id}",
                        "method": "GET"
                    },
                    {
                        "path": "/msg/inbox/{id}",
                        "method": "PATCH"
                    },
                    {
                        "path": "/msg/inbox/{id}",
                        "method": "DELETE"
                    },
                    {
                        "path": "/msg/inbox/read-all",
                        "method": "POST"
                    },
                    {
                        "path": "/msg/unread-count",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:dictionary:create": {
                "btn": [
                    "admin:btn:system:dictionary:create"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary",
                        "method": "POST"
                    }
                ]
            },
            "admin:system:dictionary:delete": {
                "btn": [
                    "admin:btn:system:dictionary:delete"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:user-group:create": {
                "btn": [
                    "admin:btn:system:user-group:create"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group",
                        "method": "POST"
                    }
                ]
            },
            "admin:system:user-group:delete": {
                "btn": [
                    "admin:btn:system:user-group:delete"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:audit:login:export-report": {
                "btn": [
                    "admin:btn:audit:login:export-report"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/login/export",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:reset-query": {
                "btn": [
                    "admin:btn:system:action:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action",
                        "method": "GET"
                    },
                    {
                        "path": "/action/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/action/group",
                        "method": "GET"
                    },
                    {
                        "path": "/action/catalog",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:view-detail": {
                "btn": [
                    "admin:btn:system:action:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:dictionary:refresh": {
                "btn": [
                    "admin:btn:system:dictionary:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary",
                        "method": "GET"
                    },
                    {
                        "path": "/dictionary/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user-group:refresh": {
                "btn": [
                    "admin:btn:system:user-group:refresh"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group",
                        "method": "GET"
                    },
                    {
                        "path": "/user-group/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:msg:delete-selected": {
                "btn": [
                    "admin:btn:system:msg:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/msg/sent/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:audit:operation:reset-query": {
                "btn": [
                    "admin:btn:audit:operation:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/operation",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/operation/summary",
                        "method": "GET"
                    },
                    {
                        "path": "/audit/operation/option",
                        "method": "GET"
                    }
                ]
            },
            "admin:audit:operation:view-detail": {
                "btn": [
                    "admin:btn:audit:operation:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/operation/detail/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:role:delete-selected": {
                "btn": [
                    "admin:btn:system:role:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/role/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:user:delete-selected": {
                "btn": [
                    "admin:btn:system:user:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:user:unlock-password": {
                "btn": [
                    "admin:btn:system:user:unlock-password"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:audit:operation:export-report": {
                "btn": [
                    "admin:btn:audit:operation:export-report"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/audit/operation/export",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:action:delete-selected": {
                "btn": [
                    "admin:btn:system:action:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/action/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:dictionary:reset-query": {
                "btn": [
                    "admin:btn:system:dictionary:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary",
                        "method": "GET"
                    },
                    {
                        "path": "/dictionary/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:dictionary:view-detail": {
                "btn": [
                    "admin:btn:system:dictionary:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user-group:add-members": {
                "btn": [
                    "admin:btn:system:user-group:add-members"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:user-group:reset-query": {
                "btn": [
                    "admin:btn:system:user-group:reset-query"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group",
                        "method": "GET"
                    },
                    {
                        "path": "/user-group/summary",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user-group:view-detail": {
                "btn": [
                    "admin:btn:system:user-group:view-detail"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}",
                        "method": "GET"
                    }
                ]
            },
            "admin:account:profile:default-access": {
                "btn": [
                    "admin:btn:account:profile:change-password"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/auth/captcha",
                        "method": "POST"
                    },
                    {
                        "path": "/auth/captcha/verify",
                        "method": "POST"
                    },
                    {
                        "path": "/auth/change/pwd",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:user-group:view-members": {
                "btn": [
                    "admin:btn:system:user-group:view-members"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}/member",
                        "method": "GET"
                    }
                ]
            },
            "admin:system:user-group:remove-members": {
                "btn": [
                    "admin:btn:system:user-group:remove-members"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}",
                        "method": "PATCH"
                    }
                ]
            },
            "admin:system:dictionary:delete-selected": {
                "btn": [
                    "admin:btn:system:dictionary:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/dictionary/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:user-group:delete-selected": {
                "btn": [
                    "admin:btn:system:user-group:delete-selected"
                ],
                "grpc": [],
                "http": [
                    {
                        "path": "/user-group/{id}",
                        "method": "DELETE"
                    }
                ]
            },
            "admin:system:developer:compare-online": {
                "btn": [
                    "admin:btn:system:developer:compare-online"
                ],
                "http": [
                    {
                        "method": "GET",
                        "path": "/dictionary"
                    }
                ],
                "grpc": []
            },
            "admin:system:developer:refresh": {
                "btn": [
                    "admin:btn:system:developer:refresh"
                ],
                "http": [
                    {
                        "method": "GET",
                        "path": "/dictionary"
                    },
                    {
                        "method": "GET",
                        "path": "/action/summary"
                    }
                ],
                "grpc": []
            },
            "admin:system:developer:publish": {
                "btn": [
                    "admin:btn:system:developer:publish"
                ],
                "http": [
                    {
                        "method": "GET",
                        "path": "/dictionary"
                    },
                    {
                        "method": "PATCH",
                        "path": "/dictionary/{id}"
                    }
                ],
                "grpc": []
            },
            "admin:system:action:create-group": {
                "btn": [
                    "admin:btn:system:action:create-group"
                ],
                "http": [
                    {
                        "method": "GET",
                        "path": "/dictionary"
                    },
                    {
                        "method": "PATCH",
                        "path": "/dictionary/{id}"
                    }
                ],
                "grpc": []
            },
            "admin:system:action:reorder-groups": {
                "btn": [
                    "admin:btn:system:action:reorder-groups"
                ],
                "http": [
                    {
                        "method": "GET",
                        "path": "/dictionary"
                    },
                    {
                        "method": "PATCH",
                        "path": "/dictionary/{id}"
                    }
                ],
                "grpc": []
            },
            "admin:system:action:delete-group": {
                "btn": [
                    "admin:btn:system:action:delete-group"
                ],
                "http": [
                    {
                        "method": "GET",
                        "path": "/dictionary"
                    },
                    {
                        "method": "PATCH",
                        "path": "/dictionary/{id}"
                    }
                ],
                "grpc": []
            }
        },
        "defaults": {},
        "resources": {
            "admin:audit": {
                "kind": "directory",
                "sort": 30,
                "child_order": [
                    {
                        "key": "admin:audit:login",
                        "kind": "node"
                    },
                    {
                        "key": "admin:audit:operation",
                        "kind": "node"
                    }
                ]
            },
            "admin:system": {
                "kind": "directory",
                "sort": 20,
                "child_order": [
                    {
                        "key": "admin:system:user",
                        "kind": "node"
                    },
                    {
                        "key": "admin:system:user-group",
                        "kind": "node"
                    },
                    {
                        "key": "admin:system:role",
                        "kind": "node"
                    },
                    {
                        "key": "admin:system:action",
                        "kind": "node"
                    },
                    {
                        "key": "admin:system:dictionary",
                        "kind": "node"
                    },
                    {
                        "key": "admin:system:msg",
                        "kind": "node"
                    },
                    {
                        "key": "admin:system:developer",
                        "kind": "node"
                    }
                ]
            },
            "admin:default": {
                "kind": "directory",
                "sort": 0,
                "collection": "default_permissions",
                "references": [
                    "admin:dashboard:overview",
                    "admin:dashboard:workspace"
                ],
                "child_order": [
                    {
                        "key": "admin:account:profile",
                        "kind": "node"
                    },
                    {
                        "key": "admin:msg:inbox",
                        "kind": "node"
                    },
                    {
                        "key": "admin:default:session",
                        "kind": "node"
                    },
                    {
                        "key": "admin:dashboard:overview",
                        "kind": "reference"
                    },
                    {
                        "key": "admin:dashboard:workspace",
                        "kind": "reference"
                    }
                ],
                "frontend_visible": false
            },
            "admin:dashboard": {
                "kind": "directory",
                "sort": 10,
                "child_order": [
                    {
                        "key": "admin:dashboard:overview",
                        "kind": "node"
                    },
                    {
                        "key": "admin:dashboard:workspace",
                        "kind": "node"
                    }
                ]
            },
            "admin:msg:inbox": {
                "kind": "resource",
                "sort": 20,
                "parent": "admin:default",
                "entries": {
                    "admin:menu:msg:inbox": {
                        "path": "/msg/inbox"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:msg:inbox"
                        ],
                        "button_keys": [
                            "admin:msg:inbox:default-access"
                        ]
                    }
                }
            },
            "admin:system:msg": {
                "kind": "resource",
                "sort": 60,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:msg": {
                        "path": "/system/msg",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:msg"
                        ],
                        "button_keys": [
                            "admin:system:msg:query",
                            "admin:system:msg:reset-query",
                            "admin:system:msg:refresh",
                            "admin:system:msg:view-detail"
                        ]
                    },
                    "create": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:msg:send"
                        ]
                    },
                    "delete": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:msg:delete",
                            "admin:system:msg:delete-selected"
                        ]
                    }
                }
            },
            "admin:audit:login": {
                "kind": "resource",
                "sort": 10,
                "parent": "admin:audit",
                "entries": {
                    "admin:menu:audit:login": {
                        "path": "/audit/login",
                        "parent_menu_key": "admin:menu:audit"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:audit:login"
                        ],
                        "button_keys": [
                            "admin:audit:login:query",
                            "admin:audit:login:reset-query",
                            "admin:audit:login:refresh",
                            "admin:audit:login:view-detail",
                            "admin:audit:login:export-report"
                        ]
                    }
                }
            },
            "admin:system:role": {
                "kind": "resource",
                "sort": 30,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:role": {
                        "path": "/system/role",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:role"
                        ],
                        "button_keys": [
                            "admin:system:role:query",
                            "admin:system:role:reset-query",
                            "admin:system:role:refresh",
                            "admin:system:role:view-detail"
                        ]
                    },
                    "create": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:role:create"
                        ]
                    },
                    "delete": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:role:delete",
                            "admin:system:role:delete-selected"
                        ]
                    },
                    "update": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:role:edit"
                        ]
                    }
                }
            },
            "admin:system:user": {
                "kind": "resource",
                "sort": 10,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:user": {
                        "path": "/system/user",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:user"
                        ],
                        "button_keys": [
                            "admin:system:user:query",
                            "admin:system:user:reset-query",
                            "admin:system:user:refresh",
                            "admin:system:user:view-detail"
                        ]
                    },
                    "create": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:user:create"
                        ]
                    },
                    "delete": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:user:delete",
                            "admin:system:user:delete-selected"
                        ]
                    },
                    "update": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:user:edit",
                            "admin:system:user:approve",
                            "admin:system:user:lock",
                            "admin:system:user:unlock",
                            "admin:system:user:unlock-password"
                        ]
                    }
                }
            },
            "admin:system:action": {
                "kind": "resource",
                "sort": 40,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:action": {
                        "path": "/system/action",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:action"
                        ],
                        "button_keys": [
                            "admin:system:action:query",
                            "admin:system:action:reset-query",
                            "admin:system:action:refresh",
                            "admin:system:action:view-detail"
                        ]
                    },
                    "create": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:action:create",
                            "admin:system:action:create-group"
                        ]
                    },
                    "delete": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:action:delete",
                            "admin:system:action:delete-selected",
                            "admin:system:action:delete-group"
                        ]
                    },
                    "update": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:action:edit",
                            "admin:system:action:reorder-groups"
                        ]
                    }
                }
            },
            "admin:account:profile": {
                "kind": "resource",
                "sort": 10,
                "parent": "admin:default",
                "entries": {
                    "admin:menu:account:profile": {
                        "path": "/profile"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:account:profile"
                        ],
                        "button_keys": [
                            "admin:account:profile:default-access"
                        ]
                    }
                }
            },
            "admin:audit:operation": {
                "kind": "resource",
                "sort": 20,
                "parent": "admin:audit",
                "entries": {
                    "admin:menu:audit:operation": {
                        "path": "/audit/operation",
                        "parent_menu_key": "admin:menu:audit"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:audit:operation"
                        ],
                        "button_keys": [
                            "admin:audit:operation:query",
                            "admin:audit:operation:reset-query",
                            "admin:audit:operation:refresh",
                            "admin:audit:operation:view-detail",
                            "admin:audit:operation:export-report"
                        ]
                    }
                }
            },
            "admin:default:session": {
                "kind": "resource",
                "sort": 30,
                "parent": "admin:default",
                "entries": {},
                "operations": {
                    "read": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:default:session:access"
                        ]
                    }
                }
            },
            "admin:system:developer": {
                "kind": "resource",
                "sort": 70,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:developer": {
                        "path": "/system/developer",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:developer"
                        ],
                        "button_keys": [
                            "admin:system:developer:compare-online",
                            "admin:system:developer:refresh"
                        ]
                    },
                    "update": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:developer:publish"
                        ]
                    }
                }
            },
            "admin:system:dictionary": {
                "kind": "resource",
                "sort": 50,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:dictionary": {
                        "path": "/system/dictionary",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:dictionary"
                        ],
                        "button_keys": [
                            "admin:system:dictionary:query",
                            "admin:system:dictionary:reset-query",
                            "admin:system:dictionary:refresh",
                            "admin:system:dictionary:view-detail"
                        ]
                    },
                    "create": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:dictionary:create"
                        ]
                    },
                    "delete": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:dictionary:delete",
                            "admin:system:dictionary:delete-selected"
                        ]
                    },
                    "update": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:dictionary:edit"
                        ]
                    }
                }
            },
            "admin:system:user-group": {
                "kind": "resource",
                "sort": 20,
                "parent": "admin:system",
                "entries": {
                    "admin:menu:system:user-group": {
                        "path": "/system/user-group",
                        "parent_menu_key": "admin:menu:system"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:system:user-group"
                        ],
                        "button_keys": [
                            "admin:system:user-group:query",
                            "admin:system:user-group:reset-query",
                            "admin:system:user-group:refresh",
                            "admin:system:user-group:view-detail",
                            "admin:system:user-group:view-members"
                        ]
                    },
                    "create": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:user-group:create"
                        ]
                    },
                    "delete": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:user-group:delete",
                            "admin:system:user-group:delete-selected"
                        ]
                    },
                    "update": {
                        "entry_keys": [],
                        "button_keys": [
                            "admin:system:user-group:edit",
                            "admin:system:user-group:add-members",
                            "admin:system:user-group:remove-members"
                        ]
                    }
                }
            },
            "admin:dashboard:overview": {
                "kind": "resource",
                "sort": 10,
                "parent": "admin:dashboard",
                "entries": {
                    "admin:menu:dashboard:overview": {
                        "path": "/dashboard/overview",
                        "parent_menu_key": "admin:menu:dashboard"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:dashboard:overview"
                        ],
                        "button_keys": []
                    }
                }
            },
            "admin:dashboard:workspace": {
                "kind": "resource",
                "sort": 20,
                "parent": "admin:dashboard",
                "entries": {
                    "admin:menu:dashboard:workspace": {
                        "path": "/dashboard/workspace",
                        "parent_menu_key": "admin:menu:dashboard"
                    }
                },
                "operations": {
                    "read": {
                        "entry_keys": [
                            "admin:menu:dashboard:workspace"
                        ],
                        "button_keys": []
                    }
                }
            }
        }
    }
}'::jsonb,
    'Locale registry, shared messages, auth responses, Vben UI messages and scoped permission labels.',
    TRUE
);

SELECT setval(pg_get_serial_sequence('t_dictionary', 'id'), (SELECT MAX(id) FROM t_dictionary), TRUE);

-- +migrate Down
-- Clear dictionaries and reset their sequence.
TRUNCATE TABLE t_dictionary RESTART IDENTITY;
