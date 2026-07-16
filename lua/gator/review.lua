local M = {
	name = "review",
	api_version = 1,
	actions = require("gator.review.actions"),
	evidence = require("gator.review.evidence"),
	feedback = require("gator.review.feedback"),
	inventory = require("gator.review.inventory"),
	validation = require("gator.review.validation"),
}

return M
