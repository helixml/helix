# Find-AI weekly project meeting

- **Date:** Mon, 20 Jul 2026 14:00:00 UTC
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **Candidate Sourcing Tool:** AI scores candidates for outreach; recruiters control message sending and candidate promotion to Bullhorn database.  
- **Workflow Handoff:** AI manages early sourcing; humans take over after conversation or CV, with manual promotion into Bullhorn.  
- **UI Improvements Needed:** Job-based filtering and hierarchical navigation planned; better sidebar and light/dark mode support required.  
- **Slack Integration:** Alerts for search completion and candidate approvals will enhance real-time team collaboration.  
- **LinkedIn Automation Issues:** Login limits cause lockouts; best practice is logging out of other sessions and scheduling overnight runs.  
- **Roadmap Focus:** Prioritize job filtering, Bullhorn promotion, Slack notifications, login stability, and event outreach use case expansion.

## Transcript

Leah Smith: Sa.
Luke Marsden: Hello.
Leah Smith: Hello.
Tony: Hey.
Luke Marsden: How you doing?
Leah Smith: Good, thank you.
Leah Smith: How are you?
Luke Marsden: Good, yeah, very well, thanks.
Leah Smith: Just gonna check that Tony's joining us.
Leah Smith: I. Yeah, cool.
Leah Smith: I've just messaged.
Leah Smith: Did you have a nice weekend?
Luke Marsden: Yeah, I did, yeah.
Luke Marsden: My co founder Chris is in Bristol with me at the moment.
Leah Smith: Oh, nice.
Luke Marsden: Yeah, yeah, we're having a lovely time.
Leah Smith: Is he playing with you?
Luke Marsden: Yes.
Leah Smith: Oh, that's cute.
Luke Marsden: I know, isn't it?
Leah Smith: How did you two meet?
Leah Smith: Are you like.
Leah Smith: Have you known each other for a long time?
Luke Marsden: Yeah, so he worked with me in my second startup and then we co founded this one together.
Luke Marsden: So we've, we've gone to battle together before.
Leah Smith: Have you still got your other startup then?
Luke Marsden: No, no, I've did kind of one at a time so.
Luke Marsden: So this is number three.
Luke Marsden: But yeah, nice.
Leah Smith: I have.
Leah Smith: So you.
Leah Smith: Did you go out at the weekend together?
Luke Marsden: Yeah, yeah, we actually went to a friend's birthday party and then he made loads of cocktails which is pretty fun.
Luke Marsden: So he was like a mixologist.
Leah Smith: Oh man.
Leah Smith: For the, for the evening?
Luke Marsden: Yeah, yeah, it was pretty nice.
Leah Smith: What was his speciality cocktail?
Luke Marsden: Old fashioned.
Leah Smith: I don't normally have those.
Leah Smith: Is that.
Leah Smith: Yeah, whiskey.
Luke Marsden: It's whiskey and orange and sours.
Luke Marsden: Yeah, it's really nice.
Luke Marsden: He puts quite a lot of sugar in them.
Leah Smith: So.
Leah Smith: Was he quite hungover on Sunday?
Luke Marsden: We were fine actually.
Luke Marsden: Wasn't too bad.
Luke Marsden: How was your weekend?
Leah Smith: Yeah, what did I do?
Leah Smith: I think I went to the beach on Saturday.
Luke Marsden: Oh, nice.
Leah Smith: It was quite nice.
Leah Smith: And then yesterday just is like a house admin day, you know, meal prep, the daughter's meal prep gym.
Leah Smith: Like just getting all the.
Leah Smith: Yeah, so yeah, I didn't do too much the weekend before.
Leah Smith: I. I was out at that day festival that I was saying to you.
Tony: Yeah, yeah, yeah.
Leah Smith: I was off on the Monday and it took a. I think it took like the whole week basically to recover.
Leah Smith: Yeah, I was just so tired the whole week because Ada started waking up in the night.
Leah Smith: Now I don't know why, but I.
Luke Marsden: Mean, it's not easy.
Luke Marsden: If you're gonna try and have a social life and a tiny baby at the same time.
Luke Marsden: The same time, then you're going to be knackered.
Luke Marsden: But respect, I mean, and you have to do it, you have to live your life.
Luke Marsden: Hey, Tony.
Leah Smith: Hey.
Tony: You're right.
Luke Marsden: Yeah, good.
Luke Marsden: Thanks man.
Luke Marsden: How are you?
Tony: Yeah, not too bad, thank you.
Tony: Not too bad.
Tony: Enjoying the heat?
Leah Smith: Yeah, it feels like it's actually a bit cooler but I. I don't know whether that's because we've just come climatized to the heat now and it definitely.
Luke Marsden: Is a bit more pleasant than it was.
Luke Marsden: I'm.
Luke Marsden: I'm down for that.
Luke Marsden: 26 Is my ideal temperature, I reckon.
Luke Marsden: Yeah.
Tony: Anything above that, you've got your vest on, Luke.
Luke Marsden: I know.
Luke Marsden: Oh yeah.
Luke Marsden: I literally was doing that the other day, wasn't I?
Luke Marsden: Yes.
Luke Marsden: Now I'm wearing proper clothes today for this professional meeting.
Luke Marsden: Good, good, cool.
Luke Marsden: So things are coming together quite nicely.
Luke Marsden: I can show you kind of where I'm at.
Luke Marsden: It is not all guaranteed to work first time when I demo it because it's all still a bit in flux but.
Luke Marsden: But I'm, I'm happy with the direction and I'd love your feedback.
Luke Marsden: So let me.
Luke Marsden: And just to reiterate this is a primary focus on helping your team do kind of agent assisted candidate sourcing per your guidance Tony last week of like that's the priority for you right now.
Luke Marsden: So with that said, let me.
Luke Marsden: So we now have os, we find AI and if I sign out you can see only people with Linux Recruit email addresses can sign into this.
Luke Marsden: So it's an internal tool.
Luke Marsden: Do all of your guys have Linux Recruit email addresses?
Tony: They do but we've obviously got find AI email addresses as well now.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: Okay so I'll, I'll extend it to also yeah those.
Luke Marsden: Are you making those separate email accounts?
Tony: They will be separate so everyone will have.
Tony: Everyone at the moment will have lynch groups, that's fine but there could be new people that we hire that will only have we find maybe not an issue at the moment but you know.
Luke Marsden: Yeah, yeah that's fine.
Luke Marsden: That's an easy thing to fix.
Luke Marsden: I can just make it support either so I can log in.
Luke Marsden: I'm going with my nice Linux Recruit email address and then candidate search.
Luke Marsden: This is the, the new section that I've been working on and can you see this?
Luke Marsden: Okay.
Luke Marsden: Should I make it a bit bigger?
Tony: Yeah, a little bit bigger but yeah.
Tony: Perfect.
Luke Marsden: Is that all right there?
Luke Marsden: Okay cool.
Luke Marsden: So the basic idea is you're going to be recruiting for different clients.
Tony: So.
Luke Marsden: I can put a new client in here like I don't know, Barclays and then you can upload files.
Luke Marsden: So let me just.
Luke Marsden: Do something like this.
Luke Marsden: Just making this up as I go along.
Tony: Yeah.
Luke Marsden: Let's say I want a PDF of that.
Luke Marsden: Save that to my downloads so you can upload and I, I got this idea from.
Tony: Will it, will it improve and be better than more or is it generally.
Luke Marsden: Yes.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: This is where I warned you about the life.
Luke Marsden: No.
Luke Marsden: So the upload.
Luke Marsden: And then.
Luke Marsden: Window.
Luke Marsden: Click the run button.
Luke Marsden: Of like.
Luke Marsden: Contact details for someone in.
Tony: So this is.
Tony: Is there a number?
Tony: Is there.
Tony: This is the search.
Tony: What's happened with this guy?
Tony: Sourced it on LinkedIn and then said we've actually.
Luke Marsden: Bullhorn is here.
Luke Marsden: It's not in Bullhorn, but it would have found him in Bullh.
Tony: Right.
Tony: Oh, yeah.
Luke Marsden: And then this is like.
Tony: Yep.
Luke Marsden: If it.
Luke Marsden: I need you to log into LinkedIn form.
Luke Marsden: In the right person on Slack or whatever.
Tony: Yeah.
Luke Marsden: Please find this.
Luke Marsden: And then this is why.
Luke Marsden: Why we think they're a fit.
Luke Marsden: And suggests an opener.
Tony: Where it says at the bottom there to owner Ethan Farrell, does that mean that he's already messaged that person or do you know what the context is behind that one?
Tony: The one at the bottom.
Luke Marsden: Here in mail was ever sent.
Tony: Okay, so I'm looking at the.
Tony: One of the different candidates, right?
Luke Marsden: Yeah, this is this one here.
Luke Marsden: Oh, yeah.
Luke Marsden: Route to Ethan Farrell.
Luke Marsden: Yeah, yeah, yeah.
Tony: He's already sent him a Z.
Luke Marsden: So.
Luke Marsden: So some.
Luke Marsden: So Ethan's already contacted them and moved to Ethan's private project.
Luke Marsden: It's interesting, isn't it?
Luke Marsden: It can see when Ethan looked at the profile and everything.
Luke Marsden: So, Yeah, this is the kind of thing that it's doing.
Luke Marsden: So if.
Luke Marsden: Okay.
Luke Marsden: The recruiter still hasn't signed in.
Luke Marsden: Look, the agent's getting impatient.
Luke Marsden: If.
Tony: You say.
Tony: Because obviously it's suggesting people to us at the moment.
Luke Marsden: Yeah.
Tony: We just say, look, contact these people who score.
Tony: Is it a percentage score they've given us contact or a percentage over 80 or whatever?
Tony: Is that.
Luke Marsden: Yeah, let me just check.
Luke Marsden: Sorry, I'm just doing two factor.
Luke Marsden: No, no, it's fine.
Luke Marsden: So you can see I'm doing two factor auth here.
Tony: Yeah.
Luke Marsden: And okay.
Luke Marsden: No, so now I'm in.
Luke Marsden: Yeah, I'm not entirely sure.
Luke Marsden: So now I'm just going to say I'm in.
Luke Marsden: You just hit enter twice to interrupt it and.
Luke Marsden: Yeah, I think it's.
Luke Marsden: It's scoring like zero almost.
Luke Marsden: Yeah.
Luke Marsden: Like out of one.
Luke Marsden: Yeah.
Luke Marsden: We can automate.
Luke Marsden: I think what.
Luke Marsden: What would be sensible would be comfortable with it.
Luke Marsden: Where the humans hand hold the AI a bit.
Tony: Yeah.
Luke Marsden: To check for mistakes before we dive straight in.
Luke Marsden: But as you and I and your team do that with it, then.
Luke Marsden: Yeah, absolutely.
Luke Marsden: We can start saying automate more of.
Tony: This and we have multiple searches going on at any one time.
Luke Marsden: Yes, I believe so.
Luke Marsden: So here, for example, this one's running.
Luke Marsden: Look at this one.
Luke Marsden: I can go the runs.
Luke Marsden: Yeah, yeah.
Luke Marsden: So you can open the other runs and so complaining because I never logged in.
Luke Marsden: Oh, it's so boring.
Luke Marsden: I'm waiting for this.
Luke Marsden: Huge.
Leah Smith: And then.
Luke Marsden: Yeah, these are just some other ones I was running earlier in the background.
Luke Marsden: So, yeah, this is.
Tony: This one's giving it a percentage.
Tony: That's what I saw it.
Tony: So if you look at this guy here.
Tony: Yeah, he's got a night go down.
Tony: Sorry.
Tony: So that.
Tony: That's.
Tony: I don't know.
Luke Marsden: Oh, yeah, there is a percent.
Tony: The other ones didn't have that.
Luke Marsden: Yeah, that's the difference between the action queue and the candidates.
Tony: Okay,.
Luke Marsden: So it's good.
Luke Marsden: Basic candidate queue is like a queue of candidates that haven't been added to Bullhorn yet.
Luke Marsden: I think you told me that you don't add them to Bullhorn until you've got a cv and Adam, until we've,.
Tony: You know, had a conversation or.
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: So these are candidates that we can.
Luke Marsden: Then the idea is that I haven't built this part yet, but the idea is that you then promote them into Bullhorn once you.
Luke Marsden: Or the AI is happy that you've got the cv, you've had enough of a chat that you think is worth following up on.
Luke Marsden: Oh, and look, it's even done a whole bunch of InMail drafts.
Tony: As if we want a bit more human interaction here.
Tony: I could look at it.
Tony: Yep, they all look good.
Tony: Please start sending those.
Luke Marsden: Yeah, and I think you would do that by approving them.
Luke Marsden: And you can edit this so you can change it.
Luke Marsden: And.
Luke Marsden: Yeah.
Luke Marsden: So how does this feel overall?
Tony: Yeah, this looks great.
Luke Marsden: Bullhorn.
Tony: Are we able to do the same process on Bullhorn as well?
Tony: But obviously, rather than sending a LinkedIn in mail, we send them an email about a role we've got.
Tony: Is that.
Luke Marsden: Ah, that's a good one.
Luke Marsden: I hadn't thought of that.
Tony: It will search Bullhorn and send us an alert saying, jimmy Smith looks great for this role because of xyz, I think.
Luke Marsden: Yeah, yeah, yeah.
Luke Marsden: Interesting.
Tony: Or work overnight to say, right, Tony, first five course this morning should be these people, this role, these people, this role.
Tony: Something like that.
Tony: I don't know.
Luke Marsden: No, I mean, that sounds really powerful, doesn't it?
Luke Marsden: Yeah, yeah, we can definitely do that.
Luke Marsden: We can add that in.
Luke Marsden: I. I did have in here somewhere, the ability.
Luke Marsden: I think it was called a sweep.
Luke Marsden: Like you talked last week about doing a sweep of saved searches.
Tony: Yeah, yeah.
Tony: So all the guys have.
Tony: Yeah.
Tony: Have their.
Tony: Yeah.
Tony: Their safe searches set up on LinkedIn.
Tony: Anyone pops up then.
Luke Marsden: But that's a.
Luke Marsden: That's different to what you're saying.
Luke Marsden: You're saying, like, for a given client, can you do a search of Bullhorn as opposed to a search of LinkedIn?
Tony: Yes.
Tony: So the same process you're doing here, which is.
Luke Marsden: Yeah.
Tony: The info into the.
Tony: It's kind of what.
Tony: What I think I sent a couple of chat GPT examples where I hadn't.
Luke Marsden: Looked at those yet.
Tony: Yeah, yeah, that's cool.
Tony: So if I, if I just talk you through that workflow quickly.
Tony: I've pulled out like a CSV file of.
Tony: Of candidates from Bullhorn.
Tony: The last column of that CSV file is like a plain text whole copy their cv.
Luke Marsden: Yeah, okay.
Tony: He's in there as well.
Tony: And then I've dragged in some info on that role and said, looking at this CSV file, which candidates would be suitable for this role?
Tony: And it's pushed out like, you know.
Luke Marsden: Okay, okay, okay, that makes sense.
Luke Marsden: And.
Luke Marsden: And that CSV is something you exported from Bullhorn and then added manually or.
Tony: I just do the math.
Tony: So I, I don't know whether we'd need to export a CSV file every week or whatever.
Tony: And.
Tony: Or there's a way we can just.
Luke Marsden: Do anything manually.
Luke Marsden: Or just use the Bullhorn API, which it already is.
Luke Marsden: It's currently using the Bullhorn API.
Luke Marsden: You do duplication checking.
Tony: Okay.
Luke Marsden: And look, for example, this one, literally, they just like the agent just searched for this person's name in Bullhorn and said, not in Bullhorn.
Luke Marsden: Yeah, save it to a project because we've already got the whole workflow like that you showed before lined up.
Luke Marsden: So what?
Tony: So not on Bullhorn is to ignore people who are on Bullhorn for like the Jeep reasons, Because they might be on Bullhorn, but not kind of in play, you know, in a process for us at the moment.
Luke Marsden: Right.
Tony: Cross reference of Bullhorn is to.
Luke Marsden: See.
Tony: If we've got their contact details to.
Tony: To contact them.
Luke Marsden: Yes.
Luke Marsden: Okay.
Tony: There is a DTube thing there as well.
Tony: But what I think what I'm saying is I don't want it to discount people who've maybe been in process for us six months ago or.
Tony: Yeah, yeah, yeah, two years ago.
Luke Marsden: That's a really good point.
Tony: Yeah.
Tony: It's only if they're kind of interviewing right now, if that makes sense.
Luke Marsden: Yeah.
Luke Marsden: Awesome.
Luke Marsden: Okay.
Luke Marsden: No, that makes sense.
Luke Marsden: So it's not like, ignore people who are in Bullhorn.
Luke Marsden: It's just like, be aware that they're in Bullhorn.
Luke Marsden: Don't add them to Bullhorn twice.
Luke Marsden: Yeah.
Luke Marsden: And kind of just bring in that context from Bullhorn, including, like the phone numbers and.
Tony: Yeah, they're really good.
Tony: Send us.
Tony: Send us a message saying, you know, Johnny Smith has just started looking his number, his details on Bullhorn, his phone number.
Luke Marsden: Yeah, yeah, yeah, yeah, yeah.
Luke Marsden: Okay, awesome.
Luke Marsden: Great.
Luke Marsden: So.
Tony: Looks amazing, though.
Luke Marsden: Yeah, I'm really, I'm happy that it's coming together.
Luke Marsden: Sorry.
Tony: I'm excited to have a little play around with it.
Luke Marsden: Well, I was going to say the next thing we can do is you can both try and log into it and actually try and use it.
Luke Marsden: Like I said, it's not perfect yet.
Luke Marsden: It's still very much like under construction, but it is live, so.
Luke Marsden: So do you want to share?
Tony: Look at it.
Tony: Sorry, Luke, can you share screen?
Tony: Sorry.
Tony: Because there's obviously a number of tabs down the side in the.
Tony: In the menu.
Tony: Would we just ignore most of that and just focus on candidate search initially or.
Luke Marsden: Or should we?
Luke Marsden: Yeah, yeah.
Luke Marsden: So we've actually been using it internally for our own sales.
Tony: Right.
Luke Marsden: Okay.
Luke Marsden: So seeing a little bit Helix OS stuff with Find os.
Luke Marsden: Yeah.
Tony: Okay, cool.
Tony: Yeah, that'll probably be useful.
Luke Marsden: To be honest,.
Tony: Things have.
Luke Marsden: Really, really ner.
Luke Marsden: Yeah, you could both try at the same time.
Leah Smith: What my logging is it os?
Luke Marsden: Yeah.
Luke Marsden: I'm just going to paste you the link os.
Luke Marsden: We find AI with a dash.
Tony: So I've got some people working on my drive.
Luke Marsden: That's all right.
Luke Marsden: I can't hear a thing, so it's more annoying for you than for us.
Luke Marsden: Oh, 500.
Tony: Okay.
Luke Marsden: Interesting.
Leah Smith: I mean, I think although mine's a white screen, not like a dark screen.
Leah Smith: Does that make any difference?
Luke Marsden: Oh, funny.
Luke Marsden: It worked the second time.
Tony: Yeah.
Luke Marsden: All right.
Luke Marsden: Weird.
Tony: I quite like the white screen.
Tony: I was struggling to see that black screen a little bit.
Luke Marsden: Oh, yeah.
Luke Marsden: Well, you've got a little icon at the top, the.
Luke Marsden: To switch between.
Tony: Okay.
Luke Marsden: Based on whether you're sitting outside in the sunshine or not.
Tony: Well, I think that's the point.
Tony: I've got a bit of glitter.
Tony: I'm in my kitchen today, so.
Luke Marsden: Yeah, yeah, yeah, yeah, you're good.
Luke Marsden: Okay, cool.
Tony: Great.
Tony: Okay, so.
Tony: I mean, this is so simple, isn't it?
Tony: Let me.
Luke Marsden: The UI probably needs some refining and tidying up, but.
Tony: Yes, I chucked you.
Tony: Did I check you a little info about that robotics place?
Tony: Oh, here we go.
Luke Marsden: Yeah, yeah,.
Tony: I need that file.
Tony: Right.
Tony: What was that file?
Tony: Shouldn't.
Luke Marsden: I mean for testing now, you could just copy paste the overview from here into.
Tony: Okay, cool.
Luke Marsden: Because file uploads weren't working just now when I tested it, so.
Tony: Oh, okay.
Tony: Oops.
Luke Marsden: It's interesting to see you using Notebook lm.
Tony: Yeah.
Tony: This is amazing.
Tony: That one that just landed on.
Tony: Did you see that?
Tony: That was us.
Tony: School report.
Tony: So we.
Tony: We had like an online thing.
Tony: Eight minutes per call.
Tony: So I just recorded all of them.
Tony: Dragged it, dragged all the files in and then it created like a report.
Tony: So school report.
Tony: So this is a transcript.
Tony: So I just copy that in?
Tony: Yeah, yeah, that'll be better than just the notes.
Luke Marsden: Sure.
Tony: Oh, bollocks.
Tony: Sorry, I've gone too far.
Tony: Too many tabs open, haven't I?
Tony: Which one is it?
Luke Marsden: The one with no icon.
Luke Marsden: We can pick an icon for it.
Tony: New client.
Luke Marsden: We need to make it look a bit better in light mode.
Luke Marsden: It's kind of weird.
Luke Marsden: Gray on white at the moment.
Luke Marsden: That's fine.
Luke Marsden: That's.
Luke Marsden: That's.
Luke Marsden: That's enough info.
Luke Marsden: I don't even bother like editing for AI because it.
Luke Marsden: It doesn't really matter.
Luke Marsden: But yeah, cool.
Luke Marsden: So try that.
Tony: That's not the right job title though.
Tony: Two Sex.
Tony: I know it won't matter, but I just want to.
Tony: AI is the right job title.
Luke Marsden: Yep.
Tony: Ah, should I try and upload that as well?
Luke Marsden: Oh, the uploads aren't working yet.
Tony: Oh, it's not working.
Tony: Okay, cool.
Luke Marsden: Yeah, yeah.
Luke Marsden: So just start with.
Luke Marsden: With what you've got but I'll give you a shout when it's.
Tony: I'm gonna copy the job specking as well.
Luke Marsden: Oh yeah, yeah, yeah, that's good.
Luke Marsden: And make that text box a bit bigger.
Tony: Sorry.
Luke Marsden: No, no, don't be sorry.
Luke Marsden: This is good.
Luke Marsden: This is good.
Luke Marsden: User testing.
Tony: There we go.
Luke Marsden: Yeah, cool.
Luke Marsden: And then just try Run Run.
Tony: I assume I need to log into my LinkedIn.
Luke Marsden: It will ask you to.
Luke Marsden: Yeah, yeah, just give it.
Luke Marsden: Oh, what happened there?
Luke Marsden: Yeah, try reconnect.
Luke Marsden: Okay.
Luke Marsden: Yeah.
Luke Marsden: We've had two instances of second time lucky.
Luke Marsden: The login and then the video stream.
Luke Marsden: They just give that a second because it's just kind of booting up.
Luke Marsden: But if you scroll down below that I guess you don't have.
Luke Marsden: Oh yeah, you can see the runs and you can.
Luke Marsden: Oh look, those other candidates are ones that probably came in from my hypothetical Barclays.
Tony: Yeah, yeah.
Luke Marsden: One.
Tony: So what.
Tony: What can the key suggested candid stage and find the rest for last mile advanced stage.
Tony: Where would these guys have would.
Tony: These would be from previous searches that have been run in the agent.
Tony: Is that.
Luke Marsden: Yeah, yeah.
Luke Marsden: And so we probably want to filter them by job, don't we in the UI here.
Luke Marsden: Otherwise they're all mixed up together.
Leah Smith: Might be confusing as to what your.
Leah Smith: What candidates they.
Leah Smith: They apply to.
Luke Marsden: Yeah, yeah.
Luke Marsden: I think Maybe we need to kind of restructure this bit.
Luke Marsden: So this is.
Luke Marsden: This currently is just like everything in one page.
Luke Marsden: Yeah, probably it makes more sense to be like, okay, I'm going to go into job or into a client and then into the job.
Luke Marsden: The JD for that client anyway.
Leah Smith: Yeah, I think that would be good.
Luke Marsden: Oh, I'm not a robot.
Luke Marsden: It's so funny because we're like, oh.
Tony: My God, is that a bus?
Tony: Is that part of the bus?
Tony: Yeah, let go as well.
Tony: Oh my God.
Tony: Bicycle.
Luke Marsden: It's funny because it's like.
Luke Marsden: Yeah,.
Tony: Really testing me, isn't it?
Luke Marsden: It's how.
Luke Marsden: How much patience have you got?
Tony: Bridges.
Tony: All right, can anyone see a bridge?
Luke Marsden: There's a bridge.
Tony: Is it any more bridges?
Luke Marsden: That bridge, if it helps, you can full screen it to make it big.
Tony: That one?
Luke Marsden: Yeah, yeah.
Tony: Is that bridge?
Luke Marsden: Yeah.
Luke Marsden: How many humans does it take to solve a capture?
Tony: Yes.
Tony: The password is wrong.
Luke Marsden: I guess that might be why it.
Tony: I'm gonna have to do a forgot password.
Tony: It auto signs me in.
Luke Marsden: Yeah.
Luke Marsden: Cool.
Tony: Then it will give me a code.
Luke Marsden: Oh God.
Tony: God.
Tony: There we go again.
Tony: Cars go.
Tony: 60.
Luke Marsden: The recruiter is working through a security checkpoint.
Tony: Very slowly.
Luke Marsden: Where the robots start getting impatient with us.
Luke Marsden: Yeah, that's okay.
Tony: Signing request.
Tony: Right, we go.
Tony: Sorry, two sec.
Tony: Yes, it's me.
Luke Marsden: This is good though because at this point it really is the mechanical suit on.
Tony: New part.
Tony: I know what it's going to say.
Luke Marsden: But you've already used your password.
Tony: You can't use a password.
Tony: You're existing password.
Luke Marsden: No, didn't.
Luke Marsden: Yay.
Luke Marsden: Okay, so now you can send a message to the agent and just be like I'm in and then hit enter twice.
Luke Marsden: You're in.
Luke Marsden: Yeah.
Luke Marsden: So it knows that you prefer keyword search over the AI search.
Luke Marsden: And then it is a bit slower like using the browser than humans are, but it can do it in the background for you and it is.
Luke Marsden: Has infinite patience.
Luke Marsden: So.
Tony: Yeah.
Tony: Yeah.
Luke Marsden: And so this is actually doing it for that magenta, isn't it?
Tony: Yeah.
Leah Smith: So all Tony had to do was obviously upload the job spec, type in the name of the company and it's just looking through LinkedIn actually uploaded the.
Tony: Transcript of the meeting with them.
Leah Smith: Yes.
Tony: Yeah.
Tony: And the job spec as well.
Luke Marsden: Yeah.
Tony: So interesting to know what is.
Tony: What is used to do this.
Tony: But I assume it's.
Tony: It would just aggregate all the info together and then use what it thinks is important.
Tony: I guess.
Luke Marsden: Yeah.
Luke Marsden: It's figuring out how to use the keyboard to press enter to get the location set.
Luke Marsden: Is wild.
Luke Marsden: Watching How AI interacts with the browser, it like writes little bits of JavaScript and executes them in order to do stuff.
Luke Marsden: Yeah.
Tony: Can I click on that?
Luke Marsden: Yeah, yeah, you could, but it would probably throw it off if you did.
Tony: Yeah, yeah.
Luke Marsden: And are you in as well, Leah?
Leah Smith: So I was just watching Tony.
Leah Smith: Let me have a look.
Leah Smith: Yes, I am.
Leah Smith: Yes, I'm in.
Leah Smith: I haven't put any search in, but I'm in.
Luke Marsden: Yeah, yeah, yeah.
Tony: Cool.
Luke Marsden: Okay, great.
Luke Marsden: Yeah, if you go to candidate search, like after the meeting or something, if you've got time, try it out with the real job.
Luke Marsden: And yeah, both of you just dump feedback at me in Slack based on the user experience here.
Luke Marsden: Because obviously this is all very raw and it's only just started working.
Leah Smith: But so when.
Tony: What?
Leah Smith: Might be a bit of a silly question, but when obviously.
Leah Smith: So Tony's logged in as Tony and the agent is obviously running loads of searches and then it will come up in the list below of all the candidates recommended.
Luke Marsden: Yes.
Leah Smith: Or the agent automatically, you know, we mentioned obviously progressing them to like the next stage and adding them.
Leah Smith: Will they.
Leah Smith: Will the agent do that automatically in addition to obviously giving you the list of the emails that they're going to send them and stuff like that on the agent as well.
Leah Smith: Is it.
Leah Smith: Does that make sense?
Luke Marsden: Yeah.
Luke Marsden: Do you mean progressing them in Bullhorn on.
Leah Smith: So if we wanted to in mail a selection of them and then obviously they're replied, then we move them to the next stage, would that appear automatically in Tony's LinkedIn?
Leah Smith: Is it all logged on LinkedIn in addition to the agent?
Luke Marsden: Yeah, where he.
Luke Marsden: Where the agent has used Link.
Luke Marsden: Where the Agent has used LinkedIn, it has used LinkedIn as Tony.
Luke Marsden: And so it will all be there in LinkedIn recruiter.
Luke Marsden: Yeah, like all of those actions or whatever their impact is.
Luke Marsden: I guess I. I guess open question, like, how do you want to represent the pipeline?
Luke Marsden: Should Bullhorn be the source of truth for the pipeline?
Luke Marsden: Do you want find OS to have its own version of the pipeline?
Luke Marsden: Do you want to completely lean on LinkedIn recruiters database?
Tony: That's a good question.
Tony: At the moment, the guys are kind of managing their desk in two places, which isn't the most ideal.
Tony: They're obviously using LinkedIn for outreach and stuff like that and managing conversations in there with projects.
Tony: But they're also using Bullhorn.
Tony: Yeah, we've always been kind of very much everything should be on Bullhorn and everything should be managed in one place.
Tony: You know, you have your vacancies and you shortlist your candidates and all that kind of stuff.
Tony: But obviously we don't.
Tony: We aren't able to do that until we've got, like a proper CV and had a conversation with them.
Luke Marsden: Yep, yep.
Tony: We can export LinkedIn files and put them into Bullhorn, but I don't know if there's that.
Tony: There kind of is a point to doing that because in the future we might not have LinkedIn recruiter or.
Luke Marsden: Yeah.
Tony: Block the use of AI like this at some point.
Tony: Which means the more data we've got on Bullhorn, the better.
Tony: That could be another project that we run, which is basically go on LinkedIn and try and scrape as many CV profiles as you can and put them on Bullhorn.
Tony: You know, I don't know if I've answered your question there, but really everything should be.
Tony: Should be on Bullhorn once we've got to the conversation.
Tony: But that's cool.
Tony: Before we get to that, we should be organizing it all in LinkedIn in projects.
Luke Marsden: Yeah, okay.
Luke Marsden: Yeah, yeah, that makes sense.
Leah Smith: Would there be something where at the point where you've had the conversation and you've got an actual CV, not just a LinkedIn CV, that the agent recognizes it and then uploads it to Bullhorn?
Leah Smith: I don't.
Tony: You.
Leah Smith: I guess you.
Luke Marsden: Yeah.
Luke Marsden: So for me, the next step here and what we haven't.
Luke Marsden: What we haven't got yet, is that sort of promotion into Bullhorn.
Leah Smith: Yeah.
Luke Marsden: Where we will actually write a new record into the Bullhorn.
Luke Marsden: So far, the only things that we.
Luke Marsden: That the agents can do is like, reading out of Bullhorn.
Luke Marsden: But yeah, we can have like a process because if you scroll down, basically, Tony, if you look.
Luke Marsden: Yeah.
Luke Marsden: This candidate queue kind of is.
Luke Marsden: Is the list of candidates that might get promoted into Bullhorn.
Luke Marsden: So we're kind of building up a database of that inside Find os.
Tony: How would we.
Tony: How would.
Tony: How would they promote these guys into Bullhorn?
Luke Marsden: Well, we can't yet that.
Luke Marsden: Because that's like the next step.
Tony: Because they need to have a CV or.
Luke Marsden: Oh, that's okay.
Luke Marsden: It's more like, what's the workflow that we want to build?
Luke Marsden: If that makes sense.
Luke Marsden: And probably that's a.
Luke Marsden: There's some sort of button on here where the human or maybe the agent decides, oh, yeah, we're at the point of, we've got the cv, we've had the chat with them, we click a.
Luke Marsden: We click.
Luke Marsden: Click the button and then we push it into Bullhorn.
Luke Marsden: And I for that.
Luke Marsden: I'm actually thinking of doing that programmatically rather than with an agent, because we don't want the agents to be able to write kind of willy nilly into your database in case they up.
Luke Marsden: So it's probably better for that to be a human gated action.
Luke Marsden: Yeah.
Tony: I feel the whole point of the agent is to get to that point.
Luke Marsden: Yeah.
Tony: Got to that point.
Tony: We then take control of the whole process.
Luke Marsden: That makes sense.
Luke Marsden: Yeah.
Tony: That's when there's a passover between the agent and us.
Tony: That's.
Tony: That's the way I've always thought about as soon as we get.
Tony: Yeah, I'm happy to jump on a call or here's my cv, it might.
Luke Marsden: Be over to the humans.
Tony: Yeah.
Luke Marsden: And this is what I'm thinking with sales work as well, is like.
Luke Marsden: And there's so many parallels between what we're trying to use this for and what you are, which is really nice because at the point at which we need to have a sales conversation with a human, there's no way an agent's going to do that for us.
Tony: No.
Luke Marsden: And we don't want them to.
Luke Marsden: But it's all about filling our diary with those meetings or those demos or whatever.
Luke Marsden: So.
Tony: Yeah.
Luke Marsden: Yeah.
Luke Marsden: Awesome.
Luke Marsden: Okay.
Luke Marsden: I think we're aligned on that.
Luke Marsden: Yeah.
Tony: So I guess, you know, your.
Tony: Your use cases, meetings and demos.
Tony: Ours at the moment is phone calls with candidates.
Tony: So it's the same.
Tony: Same kind of.
Leah Smith: That's.
Tony: That to me is a cutoff point rather than.
Luke Marsden: Got it.
Tony: Yeah.
Tony: Cool.
Luke Marsden: Okay, cool.
Luke Marsden: I mean, I'll keep working on this.
Luke Marsden: Yeah, sorry.
Tony: L. I've got this.
Tony: Got this going on.
Tony: Okay.
Tony: So they're creating a project now and they're going to add him, is that right?
Luke Marsden: I think so.
Tony: Okay, cool.
Tony: All right.
Tony: And then is it going to start messaging people, sending in mail?
Tony: This guy.
Luke Marsden: It doesn't actually send any InMails yet.
Tony: No.
Luke Marsden: It drafts the InMails and then asks you to approve them.
Tony: Okay.
Tony: Yeah.
Luke Marsden: I don't know if you want to.
Luke Marsden: Do you want to have like a toggle where you start out approving them manually and then you get comfortable with it and then you say go into full autopilot or something.
Tony: Yeah, sounds good.
Luke Marsden: Yeah.
Luke Marsden: Okay.
Luke Marsden: And is there anything like you want to limit the number of them?
Luke Marsden: Oh, sorry.
Tony: I've got these here.
Luke Marsden: Yeah, exactly.
Tony: Sorry, sorry.
Tony: So that's this guy that.
Tony: So these.
Tony: These candidates.
Tony: So they're not in the project because they've only just set up the project.
Luke Marsden: I think some of them were probably from the previous tests I was running.
Luke Marsden: And that's where it's confusing that this candidate search shows, like all of the candidates across all of the jobs.
Luke Marsden: And really we need like multi level pages where you go into a client, you go into a job and then you get the list that's filtered to that.
Luke Marsden: So I'll sort that out like as the next pass on this so it's less confusing but it would be interesting.
Luke Marsden: Yeah.
Luke Marsden: So if you look at that.
Luke Marsden: Yeah, yeah.
Luke Marsden: There's the draft.
Tony: Can we.
Luke Marsden: Oh, that's the Barclays one.
Luke Marsden: Like the made up one that I just invented.
Luke Marsden: Yeah.
Luke Marsden: So we don't want to send that.
Luke Marsden: It's an imaginary job.
Luke Marsden: Yeah.
Tony: But it's similar to the.
Tony: These candles are still good for loads of roles.
Tony: We've got to be totally honest.
Luke Marsden: Yeah.
Tony: Is there a way I can.
Tony: There's a filter.
Tony: Have.
Luke Marsden: It's so funny watching you explore this because, like I didn't even know we built that bit of ui.
Luke Marsden: The agent did it.
Tony: That.
Tony: That could be filled if you could put in the filters job.
Leah Smith: Yeah.
Tony: Which job it is.
Tony: Or whatever.
Tony: And then you could.
Luke Marsden: Yeah, yeah.
Luke Marsden: Or like I was just thinking you kind of go into a job and then once you're.
Luke Marsden: Once you're in the job, then you only see the stuff that's relevant to it.
Luke Marsden: Yeah.
Luke Marsden: Either way.
Tony: Yeah.
Tony: Yeah.
Luke Marsden: Cool.
Luke Marsden: So.
Luke Marsden: Yeah.
Luke Marsden: And it is giving them all scores and 0% poor.
Luke Marsden: Praveen.
Tony: Right.
Tony: So I'll just leave that to do its thing for a little bit and.
Luke Marsden: Then basically leave it running.
Tony: Yeah.
Tony: And.
Luke Marsden: Yeah.
Luke Marsden: And I think the other thing is that once the Slack integration is plugged in, then you'll start getting.
Luke Marsden: You'll find that these agents start being chatty with you on there.
Leah Smith: Okay.
Luke Marsden: And then they'll be like, oh, I finished this run.
Luke Marsden: Come and review the list or something like this and they'll send you a link to what you need to do next.
Luke Marsden: That's the idea anyway.
Tony: Amazing.
Tony: Yeah, that's good.
Luke Marsden: All right.
Tony: Yeah.
Tony: Should I just give you a bit of an update on.
Luke Marsden: Please.
Tony: Yeah.
Tony: And will this.
Tony: If I close my laptop down, it.
Luke Marsden: Carries on running in the background.
Tony: Say this agent's just going to be building me a shortlist and pipelining them into a project.
Luke Marsden: Yes.
Tony: Yeah.
Tony: Sweet.
Tony: Okay.
Tony: Until I tell it to stop or.
Luke Marsden: Good question.
Luke Marsden: Until it gets bored.
Luke Marsden: I mean, open question.
Luke Marsden: Do you want to fit.
Luke Marsden: Fit.
Luke Marsden: Fix the number of candidates?
Luke Marsden: No.
Luke Marsden: Maximum.
Tony: Or I guess if it builds a project and shortlists 10, 000 people, then that's probably the problem as well, isn't it?
Luke Marsden: Too much.
Tony: I'd rather have too many than not as many.
Tony: Maybe I could then just say.
Tony: Okay, You've.
Tony: You've pipelined it down to this.
Tony: Can you pipeline it even.
Tony: Even further?
Luke Marsden: Yeah.
Luke Marsden: Well, here it's.
Luke Marsden: It's restricted it down to 147, so that seems reasonable.
Tony: Search.
Tony: It's just changed its search by the looks of it.
Leah Smith: And you only wanted people above the 0.8, didn't you, Tony?
Leah Smith: Or something as well.
Tony: Yeah, but it won't know that until it goes onto the profile.
Tony: So we go on the profile and it will, I assume, scan the profile and say, okay, this person is good or not.
Tony: Is it going to go.
Tony: Do you remember we told it to go through one by one?
Luke Marsden: Yeah, we did.
Luke Marsden: And it seems to not be doing that with the left and right buttons like you wanted, but instead it goes back to the list.
Luke Marsden: But it's probably fine.
Luke Marsden: I think it's probably going to get the job done.
Tony: Yeah.
Tony: It doesn't matter how it does it, really.
Luke Marsden: Exactly.
Luke Marsden: Sausage is made.
Tony: Exactly.
Leah Smith: So once you've put.
Leah Smith: Once Tony tells the agent to stop searching, how do you then.
Leah Smith: How does the agent then start flicking through the profiles and then adding them to the.
Leah Smith: That's the action.
Leah Smith: It's in there.
Leah Smith: The action queue.
Luke Marsden: Yeah.
Luke Marsden: I mean, how would you like it to work?
Luke Marsden: What's the ideal process?
Leah Smith: What do you think, Tony?
Tony: Sorry, what was the.
Tony: What was the question?
Leah Smith: When the eight.
Leah Smith: When you say to the agent, if that's what you want to do, like, stop searching or it reaches the limit of.
Leah Smith: Yeah, the shortlist, do we then want it to, like, what's the next in the workflow?
Leah Smith: Do we then want it to start running in the background, going through the profiles of the people on the shortlist and building out, or how do we.
Tony: I think it's already done.
Tony: I think it's already done that.
Tony: So it's going on to the profiles and deciding whether they're suitable or not, and then.
Tony: Then it's pipelining them.
Tony: And then we will just have to approve the.
Tony: These drafts.
Tony: And as soon as we do that, it'll go through and send those messages to people.
Luke Marsden: Worth checking whether it actually does that, because I've never tested whether it.
Luke Marsden: Click the approve button.
Luke Marsden: Yeah.
Luke Marsden: Make sure you pick one.
Luke Marsden: That's for an actual job.
Tony: I mean, it's probably better than most emails.
Tony: All right, so this guy.
Tony: What's his name?
Tony: Praveen Dimoir.
Tony: Looks pretty decent.
Tony: Good.
Tony: Little in mail.
Luke Marsden: Yeah.
Tony: Approve edited.
Tony: Yeah.
Luke Marsden: Let's try it.
Tony: Bosch.
Tony: Okay, it's disappeared.
Tony: That's disappeared.
Tony: Now say that's engagement queue.
Leah Smith: So where is the left maybe, or not yet.
Tony: Which one?
Leah Smith: On the left.
Leah Smith: You know, you've got outreach actions, but you go into Pipeline there.
Luke Marsden: No, I don't think it does, but honestly, this is testing the limit of what we've just put together.
Luke Marsden: So I leave it with me to, like, actually find out whether that works yet.
Tony: I can look in my LinkedIn.
Luke Marsden: Yeah, yeah.
Luke Marsden: Why don't you go and look in your actual LinkedIn as well?
Tony: The only problem I can see is.
Tony: Oh, bonus.
Tony: Sorry.
Luke Marsden: Is you change your password.
Tony: Yeah, yeah, yeah, I did.
Tony: Yeah.
Luke Marsden: Weird.
Luke Marsden: Try refreshing this page before clicking Sign in or go back to LinkedIn.
Tony: There you go.
Tony: It's gonna help me in here.
Tony: The only problem.
Luke Marsden: Come on.
Tony: This is really annoying.
Tony: LinkedIn.
Tony: Link didn't get funny about you being locked in two places in one go.
Luke Marsden: Yeah.
Luke Marsden: I wonder if that's what's causing the.
Luke Marsden: Something unexpected happened.
Tony: I'll close that down for now.
Tony: I don't want to mess up the search, but I'm pretty sure I have to go on LinkedIn on my phone.
Luke Marsden: Yeah, yeah, yeah, go on your phone.
Luke Marsden: Because you'll already be logged in there, won't you?
Tony: Don't have LinkedIn recruit my phone.
Tony: It should also.
Tony: Yeah, it won't let me log in.
Luke Marsden: Really?
Tony: Yeah.
Luke Marsden: Oh, interesting.
Luke Marsden: Okay, well, can you.
Luke Marsden: Can you go to.
Luke Marsden: If you click on the desktop, like, just click on the little window behind the browser.
Tony: I'm logging in with Google, see if that works.
Tony: So I just got to go to my door quickly.
Luke Marsden: Yeah.
Luke Marsden: Sorry.
Leah Smith: This looks really cool, by the way.
Luke Marsden: Thank you.
Leah Smith: Really good.
Luke Marsden: Yeah, it's fun that it's working so well.
Luke Marsden: Yeah.
Luke Marsden: Like, just Chris earlier was like.
Luke Marsden: Don't be.
Luke Marsden: Don't.
Luke Marsden: Don't sound so surprised, Luke.
Leah Smith: I'm just thinking, like, how.
Leah Smith: Obviously I'll test it out.
Leah Smith: I'll have to find a job, use the test.
Luke Marsden: Yeah, no worries.
Tony: Yeah.
Tony: Cheers.
Leah Smith: I'll have to, like.
Leah Smith: Yeah.
Leah Smith: Use a test job or something.
Luke Marsden: Yeah, yeah.
Luke Marsden: Or you can try recruiting me for something if you like.
Leah Smith: Yeah.
Leah Smith: But, yeah, it's really good.
Leah Smith: Almost, like, spooky in a way, isn't it?
Leah Smith: The AI is, like,.
Luke Marsden: It's capable of, like, doing a lot.
Luke Marsden: Yeah.
Luke Marsden: Although it seems to be struggling a bit with this, like, existing project thing, But where it get.
Luke Marsden: Where it struggles with things, we can take that feedback and figure out how to drive it better.
Leah Smith: Is it hard for you to edit that?
Leah Smith: I don't know if edit's the right word.
Leah Smith: Is it hard for you to work on the agent in the background and tell, like, how does.
Leah Smith: I wouldn't even know what to do?
Leah Smith: How do you like do it?
Luke Marsden: I mean we're basically just.
Luke Marsden: I mean what I'm doing here is kind of building the system around the agent.
Luke Marsden: So like the, like the, the concept of clients and jobs that you configure in this app and then those just feed into these runs.
Luke Marsden: As an, as an aside, I guess you're also going to want to like run this like automatically every day probably rather than user having to come in and click Go in the morning, probably you want it to have like.
Luke Marsden: Well, you might need the user to log in, so you probably do it at like 9am or whenever.
Leah Smith: Actually there is something I could do.
Leah Smith: Well, there's a use case that I could use this agent for.
Leah Smith: We've got an event next Thursday and I use, I normally use LinkedIn Recruiter to find people that work within AI in London and I send them an email and just ask if they want to come to the next event.
Leah Smith: So I don't know whether I can test it out that way.
Luke Marsden: Yeah, yeah, I think so.
Luke Marsden: I'm just wondering how is that something you do quite regularly?
Luke Marsden: Because you put on a lot of events, don't you?
Leah Smith: Yeah, so we've got two this month, but normally it's once a month.
Leah Smith: So normally it's quite a manual task and I have to.
Leah Smith: Yeah.
Leah Smith: Physically go through or do they look like, you know.
Leah Smith: But yeah, if the agent could identify.
Leah Smith: Are they in London?
Leah Smith: Do they work in AI and draft?
Leah Smith: The InMail can be the same each.
Leah Smith: Well, same.
Leah Smith: Ish each time really.
Luke Marsden: Yeah, yeah, yeah, yeah.
Leah Smith: So those all you input into the.
Leah Smith: Instead of uploading a job spec, would you just input maybe the event information?
Luke Marsden: Yeah, I'm.
Luke Marsden: I'm thinking it's probably worth having a separate top level section for that like events that you're running and then like you're kind of sourcing attendees.
Leah Smith: Yeah.
Luke Marsden: And we can, I can just make that be a top.
Luke Marsden: Separate top level section.
Leah Smith: Yes.
Luke Marsden: Right.
Luke Marsden: Tony, should we.
Luke Marsden: You okay?
Tony: Yeah, yeah, all good.
Luke Marsden: Yeah.
Tony: I've just got something to drainage stuff on my drive.
Luke Marsden: Oh yeah.
Luke Marsden: Okay.
Luke Marsden: Should we try and get you logged into LinkedIn?
Luke Marsden: Because I, I'm a bit worried you got locked out.
Luke Marsden: Why don't we go slowly.
Luke Marsden: Let's go back to LinkedIn.com because that was an old login attempt, I think.
Luke Marsden: And then just go slowly like let's put your new password.
Tony: I'm not pressing anywhere.
Tony: Sorry, I'm not pressing anything.
Luke Marsden: Oh really?
Luke Marsden: Oh, is it just going around in a loop?
Luke Marsden: LinkedIn.
Luke Marsden: Okay, go back to LinkedIn.
Luke Marsden: Dot com.
Luke Marsden: Don't use the.
Luke Marsden: Don't use the Google log.
Tony: I didn't touch it.
Tony: This is the thing.
Tony: It automatically does it.
Luke Marsden: No, no, I know.
Luke Marsden: Oh, I see.
Luke Marsden: But go back to LinkedIn.com and then you see.
Luke Marsden: Don't click on that Google.
Tony: Yeah, I'm not.
Luke Marsden: Oh, you're not?
Tony: No, no, no.
Tony: This is the thing.
Tony: It just does it.
Luke Marsden: Okay, try.
Luke Marsden: Can you use a different browser profile for a second?
Luke Marsden: I think I feel like.
Luke Marsden: Yeah, just use a different browser for a sec.
Luke Marsden: And then sign in.
Luke Marsden: And just try.
Luke Marsden: Sign in with email because I think there's something funny going on with your Google login.
Luke Marsden: Okay.
Luke Marsden: Because it might.
Luke Marsden: Because remember we sort of thrashed a bit there and like had a few.
Luke Marsden: Had a lot of failed login attempts in a short amount of time.
Tony: Yeah.
Luke Marsden: And I feel like that might have triggered it to be like, oh, someone's basically thinking that you're being hacked rather than that you're being flagged for automation.
Luke Marsden: So now.
Luke Marsden: Now you're in now.
Luke Marsden: Now it's good.
Tony: So if I go to.
Tony: Should I try and get to recruiter, though?
Luke Marsden: Yeah, yeah, yeah, yeah, yeah.
Tony: You.
Luke Marsden: You're good.
Luke Marsden: Now if that happens again, I think maybe just leave it for 10 minutes.
Luke Marsden: Yeah.
Luke Marsden: Is my sense.
Luke Marsden: Rather than like hammering it.
Tony: Waiting reply.
Tony: No.
Luke Marsden: So it didn't automatically send, but that's to be expected because I think I told it not to automatically send.
Tony: Yeah.
Tony: When you approve it is the right.
Luke Marsden: Right.
Luke Marsden: So again, like, we haven't got there yet.
Luke Marsden: As in I. I haven't got to that point of even knowing whether that part is meant to work or not.
Luke Marsden: But yeah, I will look at that this afternoon and like I said, I.
Tony: Think, well, that guy's gone now because it found someone who looked good.
Luke Marsden: Yeah.
Tony: It's an over the gone.
Tony: It looks like it's got stuck on this bit as well.
Luke Marsden: It has got.
Luke Marsden: Oh, no, it's not got stuck so much.
Leah Smith: Oh, sorry, sorry.
Tony: It's asking a question.
Luke Marsden: Yeah, yeah.
Tony: Oh, sorry.
Luke Marsden: You might want to full screen that to make it a bit more.
Tony: Right.
Tony: So magenta.
Tony: Magenta.
Tony: It's magenta.
Tony: But keywords.
Tony: Okay, cool.
Tony: Process six ones.
Tony: Save the project.
Tony: Great.
Luke Marsden: You probably want a way to be able to collapse the sidebar here so you get a bit more horizontal space.
Tony: This bit.
Luke Marsden: Yeah.
Tony: When it says rooted to owner, it'd be interesting to know what that.
Tony: What that means.
Luke Marsden: I don't think it's.
Luke Marsden: I don't think it's probably actually.
Tony: Yeah.
Luke Marsden: Pinged Ethan.
Luke Marsden: But what do you want it to.
Tony: This Guy's quite a. I'm glad the.
Tony: I always picked up on that.
Luke Marsden: That's so funny.
Tony: Yeah.
Luke Marsden: The AI is now no longer hackable.
Luke Marsden: I mean, that's probably a reason to try and hire them, to be honest.
Tony: Yeah.
Luke Marsden: I mean, it's not a bad thing.
Tony: Had a ball on phone.
Tony: Okay, that's cool.
Tony: Two things you need.
Tony: Add David Richard.
Tony: Choose existing.
Tony: It's.
Tony: But the thing is, it's.
Tony: It's set up a project.
Luke Marsden: It's just asking you for help because it got stuck on some little thing on the ui.
Tony: Right.
Tony: So log track proven is in a ready choose project.
Tony: If we want to scan up to keep.
Tony: Would you rather.
Tony: If you.
Tony: Yeah, keep working through them, but let's.
Luke Marsden: Yeah, you might want to make that.
Tony: A bit bigger Project name.
Luke Marsden: What's it saying exactly?
Luke Marsden: It's scroll up a little bit in the chat.
Luke Marsden: Add Rio and David to the project.
Tony: Yeah.
Luke Marsden: Choose existing project selector in the save rail.
Luke Marsden: Yeah.
Luke Marsden: It's weird.
Luke Marsden: It looks like they're both selected.
Tony: Yeah.
Tony: Not seen that before.
Luke Marsden: No, that's.
Luke Marsden: I mean, it somehow managed to make LinkedIn buggy.
Luke Marsden: I would maybe refresh the page inside the.
Luke Marsden: Inside the browser.
Tony: I can just close that down or.
Luke Marsden: Close that down and try adding it again.
Luke Marsden: Yeah.
Luke Marsden: Okay.
Luke Marsden: So obviously had trouble doing the automation and that's kind of just one of the things where we just need to get.
Luke Marsden: Ask the agent to figure out how it does that part and then write it down for next time so it doesn't have to figure it out every time.
Tony: Try to add an EC select automatically or straight to the project.
Luke Marsden: Yeah, But you only added one of them and it asked you to add two.
Luke Marsden: I think there was a David as well.
Luke Marsden: So you might want to say, like, take me to David's page so I can add them as well.
Luke Marsden: Or.
Luke Marsden: Or say try again to add them to the project yourself.
Luke Marsden: Because I can't be asked.
Luke Marsden: Adding humans to projects for you, robot.
Tony: This is your job.
Leah Smith: Yeah.
Tony: This is what I pay you for.
Luke Marsden: Yeah.
Luke Marsden: Pay your tokens.
Luke Marsden: Actually, I think we're paying for the tokens at the moment.
Luke Marsden: That's all good.
Tony: Do you pay every time?
Tony: Is there like a cost every time you type something or how does it work?
Luke Marsden: Yeah, it's just based on kind of quantity of words.
Luke Marsden: This isn't costing tremendously much at the moment.
Luke Marsden: I think it's all go.
Luke Marsden: It's all probably going through my Claude subscription, to be honest.
Luke Marsden: Oh.
Luke Marsden: Multiple sessions detects on this account.
Tony: That's because I logged in here.
Luke Marsden: Yeah, just close that down.
Tony: Sorry for The.
Luke Marsden: No, no, don't worry.
Luke Marsden: Have you seen that before?
Tony: No.
Luke Marsden: Okay.
Luke Marsden: It's definitely something we're going to manage.
Luke Marsden: We're gonna have to manage.
Tony: But is it.
Luke Marsden: Oh, yeah, yeah, yeah, yeah.
Luke Marsden: That's probably because they're just protecting their revenue.
Luke Marsden: They don't want multiple people sharing.
Luke Marsden: One person per license.
Tony: That is strictly true.
Luke Marsden: Yeah.
Luke Marsden: I mean, you are just one person.
Tony: I. I don't.
Tony: This, this isn't.
Tony: We're not doing anything against LinkedIn's terms of business.
Tony: I don't think.
Tony: I know they don't like this kind of thing, but we are.
Tony: We're not like scraping data or anything like that.
Tony: We are just automating searches.
Tony: We're still paying for LinkedIn recruiter.
Luke Marsden: Yeah, yeah.
Luke Marsden: I think it's generally like, sort of accepted by LinkedIn that there is some automation that happens on the platform.
Luke Marsden: But I would also be careful because obviously if you lose your LinkedIn recruiter access, that would have a big impact on you.
Luke Marsden: So it's something we just need to, like, manage together.
Luke Marsden: I think.
Tony: I think that'd be really bad if we did.
Tony: So obviously that's.
Luke Marsden: Yeah, yeah.
Luke Marsden: What I would say there is that it probably does make sense to have this sort of best practice where when you use Helix or when you use Find OS, you do start by logging out of LinkedIn Recruiter in all your other browsers and your other browser tabs.
Luke Marsden: Yeah.
Luke Marsden: And we could even add something which does a little pop up before asking you to log in that tells you to log out everywhere else.
Tony: Yeah.
Luke Marsden: Because then you really are just one human logging in through this browser to your account and then using some automation to like, click for you.
Luke Marsden: And there's other.
Luke Marsden: And I'll do a little bit more research into what else.
Luke Marsden: What other, like, precautions.
Tony: What does it base it on, Luke?
Tony: So I looked into LinkedIn in loads of different, you know, devices in my house at the same time.
Tony: Phone, my desktop, my laptop.
Tony: I'm logged in on my desktop and my laptop at the same time.
Tony: Would it be.
Luke Marsden: It's probably based on IP address.
Luke Marsden: Yeah, I was just going to say because.
Luke Marsden: Because this IP address is.
Luke Marsden: Is a server I have in my basement.
Luke Marsden: Yeah.
Luke Marsden: So it shows up as a residential IP address.
Tony: Yeah.
Luke Marsden: But it's not the same as your IP address.
Luke Marsden: So they can tell like, I'm in.
Luke Marsden: I'm in Bristol and you're in Bournemouth or whatever.
Luke Marsden: Or rather the machine you're logging in from is in Bristol.
Luke Marsden: So that's probably a reason to.
Luke Marsden: But is that going to be a pain in the ass.
Luke Marsden: If you want the agent running in the background to have to not be logged into LinkedIn in your normal browser at the same time.
Luke Marsden: It is.
Luke Marsden: Isn't.
Tony: Probably is a little bit.
Luke Marsden: Yeah.
Tony: Because obviously we want to set the agent off to do its thing and then we want to continue working in.
Tony: In the normal way at the same time.
Luke Marsden: Yeah.
Tony: But like you said, we need to be.
Tony: We need to be careful that we're not.
Tony: Yeah.
Tony: Causing the issues.
Luke Marsden: One way to solve that is that you could just open another tab in here and get whatever manual work you need to get done.
Luke Marsden: Done.
Luke Marsden: Yeah.
Luke Marsden: Inside that desktop.
Luke Marsden: It's maybe not ideal because it's a little bit.
Luke Marsden: We need to fix that full screen as well because currently it.
Luke Marsden: Full screen is the wrong thing.
Luke Marsden: It should full screen the actual desktop.
Luke Marsden: What happens if you click full screen again there, by the way?
Tony: This one.
Luke Marsden: Yeah.
Luke Marsden: That's going to take you out, isn't it?
Tony: Yeah, yeah.
Luke Marsden: Okay, just.
Luke Marsden: Yeah, yeah.
Luke Marsden: Oops.
Tony: Is this going to make it glitch?
Tony: Have a look.
Luke Marsden: It should be fine because it's only.
Luke Marsden: So the agent is only working in that first tab and it won't interfere with your second tab.
Luke Marsden: Weird that it timed out.
Luke Marsden: There we go.
Luke Marsden: Okay.
Tony: Okay, cool.
Luke Marsden: All right.
Luke Marsden: So you can do work in parallel with it, but maybe it's safer to do it in there because then it really is just one session.
Luke Marsden: All right, cool.
Tony: Great.
Luke Marsden: All right.
Luke Marsden: Yeah, I'll keep working on it and I will.
Luke Marsden: I'm mindful of the worry about LinkedIn.
Luke Marsden: Like, I've done a bit of re.
Luke Marsden: I've done quite a bit of research into how LinkedIn detect automation.
Luke Marsden: Yeah.
Luke Marsden: And one of the main things is that they.
Luke Marsden: They don't like things that like.
Luke Marsden: Well, there are ways of making the agent appear more human.
Luke Marsden: Like.
Luke Marsden: Although to be honest, if you are manually doing this job, you will actually be looking at.
Luke Marsden: You will actually look at every result, won't you?
Luke Marsden: Like you'll click on each of them in turn.
Luke Marsden: So that is a human activity.
Luke Marsden: Oh, and copy paste just worked, didn't it?
Tony: No, it didn't.
Tony: It didn't.
Luke Marsden: That's what I was just doing.
Tony: Yeah.
Tony: I was testing to see if I could copy this message into Slack to Leah.
Luke Marsden: Yeah, Internet.
Leah Smith: Right.
Luke Marsden: Try Ctrl C. Yeah, I copied text.
Luke Marsden: No, interesting.
Tony: But let me copy and paste the other way.
Luke Marsden: Yeah, okay.
Luke Marsden: That might be because of the iframe, as in the way that we've embedded the widget.
Luke Marsden: So I'll.
Luke Marsden: I'll see if we can fix that as well.
Luke Marsden: To be honest.
Luke Marsden: You probably will be okay if you also now log in from your safari.
Tony: Log into LinkedIn.
Luke Marsden: Yeah.
Luke Marsden: If you needed to, I would say you'd probably be fine.
Luke Marsden: I think.
Tony: Also I'm fine not being in it.
Luke Marsden: The other, the other way to think of this is we can think about what, what can the agent get done overnight?
Tony: Yeah, exactly.
Luke Marsden: Is another way of looking at it.
Luke Marsden: If, if, if LinkedIn really are going to be funny about, oh, each seat should really be logged in from one IP address at a time.
Luke Marsden: Then.
Tony: It's like, yeah, and I, I would say if we're sending emails through the night, the candidate will see that and probably think, well, this is an agent, but we can schedule.
Tony: Send them for the next day.
Luke Marsden: Yeah, schedule them for like 8, 55 or something.
Luke Marsden: Or whatever.
Tony: Yeah.
Luke Marsden: And then those go out and then.
Luke Marsden: Yeah, you're hammering away at your desk, like on, like working through the leads that it's given you in the morning and then you set it off in the evening to do another round and then LinkedIn's just going to be like, these people never sleep.
Tony: Yeah,.
Luke Marsden: But that's okay.
Luke Marsden: I mean, and, and again, I think you can get a lot out of like, a few hours worth of like, AI automation.
Luke Marsden: Yeah.
Luke Marsden: Anyway, so, yeah, this is all good.
Luke Marsden: This is all really good stuff and thank you.
Tony: I can't log in on my email, though.
Luke Marsden: Oh, really?
Luke Marsden: Which one?
Luke Marsden: On your phone?
Tony: Log into LinkedIn on my phone.
Leah Smith: No.
Luke Marsden: What's it saying?
Tony: Hold on, sign in, let me try again.
Tony: No, it's not letting me.
Tony: Challenge failed.
Tony: Please try again.
Tony: Huh.
Tony: I'll try Google.
Tony: Yeah, it's not.
Tony: Let me log in.
Luke Marsden: I would say let this finish and then log out from Helix and then try getting back in on your phone, like in an hour or so.
Tony: Yeah.
Luke Marsden: And, yeah, drop me a note on.
Luke Marsden: On Slack, if, If that's still being problematic.
Tony: I might, I might just close this.
Tony: Where's all the people gone?
Tony: I might just close this down and then I close it down now, if that's all right.
Luke Marsden: Yeah, of course.
Luke Marsden: Yeah, yeah, yeah, yeah, yeah.
Luke Marsden: And I can carry on testing on my account so it won't affect yours.
Tony: Yeah, that's.
Luke Marsden: So you can click that little.
Tony: Stop.
Luke Marsden: Sorry.
Tony: You've been using yours fine, haven't you?
Luke Marsden: Yeah, yeah, mine's been.
Luke Marsden: Okay.
Tony: That button.
Luke Marsden: What's that?
Tony: This button?
Luke Marsden: Yeah, yeah, you can click that button.
Tony: Yeah, I'll just do that.
Tony: That's.
Tony: That's showing us what.
Tony: Oh, I need to log out though, don't I?
Luke Marsden: It doesn't really matter.
Luke Marsden: I think you can restart it if you want to, but it's like it's turned off the computer and on again.
Luke Marsden: So just give it a second.
Luke Marsden: Yes.
Luke Marsden: And now you'll need to open Chrome.
Luke Marsden: That's it.
Luke Marsden: And then go to LinkedIn.com.
Luke Marsden: Maybe it's.
Luke Marsden: Maybe it's not even kept the session, actually.
Tony: So it doesn't look like I'm signed in.
Luke Marsden: No.
Luke Marsden: So you can leave it.
Luke Marsden: Don't worry about it.
Luke Marsden: So, yeah, you can just hit that stop button again.
Tony: Cool.
Luke Marsden: Yeah.
Luke Marsden: So leave it with me.
Luke Marsden: I'll take on board all the feedback that we've had from the call and I'll ping you when I've got an update.
Luke Marsden: I'll have a look, see if these.
Tony: Guys are in there.
Luke Marsden: Yeah, yeah.
Luke Marsden: It'd be interesting to check that they actually show up in the project.
Luke Marsden: And it was having some trouble adding them to the project, wasn't it?
Tony: Yeah, yeah, I'll have a look.
Tony: I'll leave it half an hour and then have a look.
Luke Marsden: Okay.
Luke Marsden: Wicked.
Tony: Yeah.
Luke Marsden: Good stuff.
Luke Marsden: All right.
Tony: All right.
Luke Marsden: Great collaboration.
Luke Marsden: This is exciting.
Luke Marsden: Thanks, guys.
Luke Marsden: Appreciate it.

