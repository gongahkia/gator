local M = {
	name = "review",
	api_version = 2,
	evidence = require("gator.review.evidence"),
	inventory = require("gator.review.inventory"),
	merge = require("gator.review.merge"),
	validation = require("gator.review.validation"),
}

return M
