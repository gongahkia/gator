.pragma library

.import "EmojiUnicode16.js" as EmojiUnicode16

var rows = [
    "😀|grinning|grinning_face,grin", "😃|smiley|smiling_face", "😄|smile|smiling_face_with_smiling_eyes",
    "😁|grin|beaming_face", "😆|laughing|satisfied,squinting_face", "😅|sweat_smile|grinning_sweat",
    "😂|joy|tears_of_joy,lol", "🤣|rofl|rolling_on_the_floor_laughing", "😊|blush|smiling_face",
    "😇|innocent|halo", "🙂|slightly_smiling_face|slight_smile", "🙃|upside_down_face|upside_down",
    "😉|wink|winking_face", "😌|relieved|relief", "😍|heart_eyes|love_eyes", "🥰|smiling_face_with_3_hearts|three_hearts",
    "😘|kissing_heart|kiss", "😗|kissing|kiss_face", "😙|kissing_smiling_eyes|kiss_smile",
    "😚|kissing_closed_eyes|kiss_closed_eyes", "😋|yum|delicious", "😛|stuck_out_tongue|tongue",
    "😜|stuck_out_tongue_winking_eye|winking_tongue", "🤪|zany_face|crazy_face", "😎|sunglasses|cool",
    "🤓|nerd_face|nerd", "🧐|face_with_monocle|monocle", "🤗|hugging_face|hugs,hug",
    "🤩|star_struck|star_eyes", "🥳|partying_face|party_face", "😏|smirk|smirking_face",
    "😒|unamused|unhappy", "😞|disappointed|sad", "😔|pensive|thoughtful", "😟|worried|concerned",
    "😕|confused|unsure", "🙁|slightly_frowning_face|slight_frown", "☹️|frowning_face|frown",
    "😣|persevere|persevering", "😖|confounded|frustrated", "😫|tired_face|tired", "😩|weary|weary_face",
    "🥺|pleading_face|puppy_eyes", "😢|cry|crying_face", "😭|sob|crying", "😤|triumph|huffing",
    "😠|angry|mad", "😡|rage|pout,red_face", "🤬|face_with_symbols_on_mouth|swearing",
    "🤯|exploding_head|mind_blown", "😳|flushed|embarrassed", "🥵|hot_face|overheated",
    "🥶|cold_face|freezing", "😱|scream|screaming_face", "😨|fearful|scared", "😰|cold_sweat|anxious",
    "😥|disappointed_relieved|sad_sweat", "😓|sweat|downcast_sweat", "🤔|thinking_face|thinking",
    "🤭|hand_over_mouth|giggle", "🤫|shushing_face|shush", "🤥|lying_face|liar", "😶|neutral_face|no_mouth",
    "😐|neutral_face|neutral", "😑|expressionless|blank_face", "😬|grimacing|grimace",
    "🙄|face_with_rolling_eyes|eye_roll", "😯|hushed|surprised", "😦|frowning|frown_open",
    "😧|anguished|distressed", "😮|open_mouth|wow", "😲|astonished|astonished_face",
    "🥱|yawning_face|yawn", "😴|sleeping|sleep", "🤤|drooling_face|drool", "😪|sleepy|sleepy_face",
    "😵|dizzy_face|dizzy", "🤐|zipper_mouth_face|zipper_mouth", "🥴|woozy_face|woozy",
    "🤢|nauseated_face|nauseous", "🤮|face_vomiting|vomit", "🤧|sneezing_face|sneeze",
    "😷|face_with_medical_mask|mask", "🤒|face_with_thermometer|sick", "🤕|face_with_head_bandage|injured",
    "🤑|money_mouth_face|money_face", "🤠|cowboy_hat_face|cowboy", "👿|imp|angry_devil",
    "😈|smiling_imp|devil", "💀|skull|death", "☠️|skull_and_crossbones|pirate", "👻|ghost|spooky",
    "👽|alien|ufo", "🤖|robot|bot", "💩|poop|shit,hankey,pile_of_poo",
    "👋|wave|waving_hand", "🤚|raised_back_of_hand|backhand", "🖐️|raised_hand_with_fingers_splayed|hand",
    "✋|raised_hand|stop,high_five", "🖖|vulcan_salute|vulcan", "👌|ok_hand|okay",
    "🤌|pinched_fingers|pinch", "🤏|pinching_hand|small", "✌️|v|victory,peace",
    "🤞|crossed_fingers|fingers_crossed,luck", "🫰|hand_with_index_finger_and_thumb_crossed|heart_hands",
    "🤟|love_you_gesture|ily", "🤘|sign_of_the_horns|metal,rock_on", "🤙|call_me_hand|call_me",
    "👈|point_left|left", "👉|point_right|right", "👆|point_up_2|up", "👇|point_down|down",
    "☝️|point_up|point", "👍|thumbsup|thumb_up,+1,like", "👎|thumbsdown|thumb_down,-1,dislike",
    "👏|clap|applause", "🙌|raised_hands|hooray", "👐|open_hands|open", "🤲|palms_up_together|palms_up",
    "🤝|handshake|deal", "🙏|pray|thanks,please,namaste", "✍️|writing_hand|write",
    "💅|nail_care|nails", "🤳|selfie|phone", "💪|muscle|strong,flex", "🦾|mechanical_arm|robot_arm",
    "🫶|heart_hands|love", "🫂|people_hugging|hugging_people", "👀|eyes|look,watch",
    "👁️|eye|see", "🧠|brain|think", "🫀|anatomical_heart|heart_organ", "🫁|lungs|lung",
    "🗣️|speaking_head|speak", "👤|bust_in_silhouette|person", "👥|busts_in_silhouette|people",
    "❤️|heart|red_heart,love", "🩷|pink_heart|heart_pink", "🧡|orange_heart|heart_orange",
    "💛|yellow_heart|heart_yellow", "💚|green_heart|heart_green", "💙|blue_heart|heart_blue",
    "🩵|light_blue_heart|heart_light_blue", "💜|purple_heart|heart_purple", "🖤|black_heart|heart_black",
    "🩶|grey_heart|heart_grey", "🤍|white_heart|heart_white", "🤎|brown_heart|heart_brown",
    "💔|broken_heart|heartbreak", "❣️|heart_exclamation|heart_bang", "💕|two_hearts|two_hearts",
    "💞|revolving_hearts|hearts", "💓|beating_heart|heartbeat", "💗|growing_heart|heart_growth",
    "💖|sparkling_heart|sparkle_heart", "💘|cupid|heart_arrow", "💝|gift_heart|heart_gift",
    "💟|heart_decoration|heart_decor", "💯|100|hundred,score", "💢|anger|anger_symbol",
    "💥|boom|collision,explosion", "💫|dizzy|star", "💦|sweat_drops|splash",
    "💨|dash|wind,fast", "🕳️|hole|black_hole", "💬|speech_balloon|chat,message",
    "👁️‍🗨️|eye_in_speech_bubble|eye_speech", "🗨️|left_speech_bubble|speech", "💭|thought_balloon|thought",
    "💤|zzz|sleeping", "✅|white_check_mark|check,done,success", "☑️|ballot_box_with_check|checkbox",
    "✔️|heavy_check_mark|tick", "❌|x|cross,no", "❎|negative_squared_cross_mark|cross_mark",
    "➕|heavy_plus_sign|plus,add", "➖|heavy_minus_sign|minus,remove", "➗|heavy_division_sign|divide",
    "❓|question|question_mark", "❔|grey_question|question_gray", "❗|exclamation|exclamation_mark",
    "❕|grey_exclamation|exclamation_gray", "‼️|bangbang|double_exclamation", "⁉️|interrobang|question_exclamation",
    "⚠️|warning|caution", "🚫|no_entry_sign|forbidden", "⛔|no_entry|prohibited",
    "🔴|red_circle|circle_red", "🟠|orange_circle|circle_orange", "🟡|yellow_circle|circle_yellow",
    "🟢|green_circle|circle_green", "🔵|large_blue_circle|circle_blue", "🟣|purple_circle|circle_purple",
    "⚫|black_circle|circle_black", "⚪|white_circle|circle_white", "🟤|brown_circle|circle_brown",
    "🔔|bell|notification", "🔕|no_bell|mute", "🎵|musical_note|music,note",
    "🎶|notes|music_notes", "🔥|fire|lit,flame", "✨|sparkles|sparkle,stars",
    "⭐|star|favorite", "🌟|star2|glowing_star", "🌈|rainbow|pride", "☀️|sunny|sun",
    "🌤️|mostly_sunny|sun_cloud", "☁️|cloud|cloudy", "🌧️|rain_cloud|rain", "❄️|snowflake|snow",
    "⚡|zap|lightning", "🌊|ocean|wave,water", "🌍|earth_africa|globe,world",
    "🌎|earth_americas|globe_americas", "🌏|earth_asia|globe_asia", "🌙|crescent_moon|moon",
    "🚀|rocket|launch", "✈️|airplane|plane,flight", "🚗|car|automobile", "🚕|taxi|cab",
    "🚌|bus|public_transport", "🚆|train|railway", "🚲|bike|bicycle", "🛵|motor_scooter|scooter",
    "🚶|walking|walk", "🏃|running|run", "🏠|house|home", "🏢|office|building",
    "🏫|school|education", "🏥|hospital|medical", "🏖️|beach_with_umbrella|beach,holiday",
    "🗺️|world_map|map", "⌚|watch|time", "📅|date|calendar", "🗓️|spiral_calendar|calendar",
    "⏰|alarm_clock|alarm", "⏳|hourglass_flowing_sand|waiting", "⌛|hourglass|time",
    "📌|pushpin|pin", "📍|round_pushpin|location,map_pin", "📎|paperclip|attachment",
    "📝|memo|pencil,note", "✏️|pencil2|pencil", "📖|book|read", "📚|books|library",
    "💡|bulb|idea,light", "🔍|mag|search", "🔎|mag_right|search_right", "🔒|lock|secure",
    "🔓|unlock|open_lock", "🔑|key|password", "🛠️|hammer_and_wrench|tools", "⚙️|gear|settings",
    "🖥️|desktop_computer|computer", "💻|computer|laptop", "⌨️|keyboard|typing", "🖱️|computer_mouse|mouse",
    "📱|iphone|phone,mobile", "☎️|telephone|phone", "📧|e-mail|email,mail", "📨|incoming_envelope|mail",
    "📤|outbox_tray|send", "📥|inbox_tray|receive", "📂|open_file_folder|folder", "📁|file_folder|folder",
    "🗑️|wastebasket|trash,delete", "📊|bar_chart|chart,analytics", "📈|chart_with_upwards_trend|growth",
    "📉|chart_with_downwards_trend|decline", "💰|moneybag|money,cash", "💳|credit_card|card",
    "🎁|gift|present", "🏆|trophy|winner", "🥇|first_place_medal|gold", "🎯|dart|target",
    "⚽|soccer|football", "🏀|basketball|ball", "🎮|video_game|game", "🎧|headphones|audio",
    "☕|coffee|tea,cafe", "🍺|beer|drink", "🍷|wine_glass|wine", "🍕|pizza|food",
    "🍔|hamburger|burger", "🍣|sushi|food", "🍰|cake|dessert,birthday", "🎂|birthday|cake",
    "🐶|dog|puppy", "🐱|cat|kitten", "🐭|mouse|animal", "🐼|panda_face|panda",
    "🦊|fox_face|fox", "🐻|bear|bear_face", "🦁|lion|lion_face", "🐸|frog|frog_face",
    "🐵|monkey_face|monkey", "🙈|see_no_evil|monkey_see", "🙉|hear_no_evil|monkey_hear",
    "🙊|speak_no_evil|monkey_speak", "🌱|seedling|plant,grow", "🌳|deciduous_tree|tree",
    "🌸|cherry_blossom|flower", "🌹|rose|flower", "🎉|tada|party,celebrate",
    "🎊|confetti_ball|celebrate", "🎈|balloon|party", "🎨|art|palette,paint",
    "🚩|triangular_flag_on_post|flag", "🏳️|white_flag|flag_white", "🏴|black_flag|flag_black",
    "👍🏻|thumbsup_light_skin_tone|thumb_up_light", "👍🏼|thumbsup_medium_light_skin_tone|thumb_up_medium_light",
    "👍🏽|thumbsup_medium_skin_tone|thumb_up_medium", "👍🏾|thumbsup_medium_dark_skin_tone|thumb_up_medium_dark",
    "👍🏿|thumbsup_dark_skin_tone|thumb_up_dark", "🙏🏻|pray_light_skin_tone|thanks_light",
    "🙏🏽|pray_medium_skin_tone|thanks_medium", "🙏🏿|pray_dark_skin_tone|thanks_dark"
]

var cachedEntries = null

function normalise(value) {
    return String(value || "").toLowerCase().replace(/[\s_-]/g, "")
}

function entries() {
    if (cachedEntries !== null) return cachedEntries
    const byEmoji = {}
    cachedEntries = []
    function append(row) {
        const parts = typeof row === "string" ? row.split("|") : row
        if (!parts || parts.length < 2) return
        const emoji = parts[0]
        const name = parts[1]
        const aliases = typeof row === "string" && parts.length > 2 ? parts[2].split(",") : []
        const existing = byEmoji[emoji]
        if (existing !== undefined) {
            if (existing.name !== name && existing.aliases.indexOf(name) < 0) existing.aliases.push(name)
            aliases.forEach(function(alias) {
                if (alias !== existing.name && existing.aliases.indexOf(alias) < 0) {
                    existing.aliases.push(alias)
                }
            })
            return
        }
        const entry = { emoji: emoji, name: name, aliases: aliases }
        byEmoji[emoji] = entry
        cachedEntries.push(entry)
    }
    rows.forEach(append)
    if (EmojiUnicode16.rows !== undefined) EmojiUnicode16.rows.forEach(append)
    return cachedEntries
}

function search(query, limit) {
    var needle = normalise(query)
    var matches = []
    var maximum = Math.max(1, Number(limit) || 8)
    entries().forEach(function(entry) {
        var candidates = [entry.name].concat(entry.aliases)
        var rank = 3
        for (var index = 0; index < candidates.length; ++index) {
            var candidate = normalise(candidates[index])
            if (needle.length === 0) {
                rank = Math.min(rank, index === 0 ? 1 : 2)
            } else if (candidate === needle) {
                rank = 0
            } else if (candidate.indexOf(needle) === 0) {
                rank = Math.min(rank, 1)
            } else if (candidate.indexOf(needle) >= 0) {
                rank = Math.min(rank, 2)
            }
        }
        if (rank < 3) matches.push({ emoji: entry.emoji, name: entry.name, aliases: entry.aliases, rank: rank })
    })
    matches.sort(function(left, right) {
        if (left.rank !== right.rank) return left.rank - right.rank
        return left.name.localeCompare(right.name)
    })
    return matches.slice(0, maximum)
}
