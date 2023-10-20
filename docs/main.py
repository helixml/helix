
import sd_scripts # sdxl / image
import axolotl # mistral / text


# connect to remote server

get_next_instruction(filter=None)
dispatch_instruction() # into sd_scripts or axolotl


#######

# INSIDE sd_scripts/axolotl (where we dispatch into)

get_next_instruction(filter={"mode": "inference", "type": "text"}, timeout=300)

def get_next_instruction(filter):
	for 300 seconds, only accept text inference
	after that, get_next_instruction(filter=None), if we get a text inference, accept it and process it
	if we get ANY other instruction after the timeout, exit()
	if there are no jobs, keep running (with gpu memory nicely held)


## XX this design won't work, what we need to achieve is a gpu worker stays online until there's another competing job that the top level would look for
