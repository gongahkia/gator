local M = {
	name = "review",
	api_version = 1,
	feedback = require("gator.review.feedback"),
	inventory = require("gator.review.inventory"),
	validation = require("gator.review.validation"),
}

return M
