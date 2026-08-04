# Find-AI weekly project meeting

- **Date:** Mon, 15 Jun 2026 14:00:00 UTC
- **Duration:** 21 min
- **Participants:** luke@helix.ml, chris@helix.ml, leah.smith@linuxrecruit.co.uk, tony.chapman@linuxrecruit.co.uk, luke@mlops.consulting
- **Source:** fireflies

## Summary

- **Website framework:** Basic job search and branding site built; simple copy, clear calls to action; Insights page delayed due to content gaps.

- **Content strategy:** Manual content for now; future plan for automated blog/news feeds and AI tools for easy team updates.

- **Job search rollout:** Interim integration with Linux Recruit’s Bullhorn API; phased introduction of AI job matching and profile features.

- **AI suitability tool:** Prototype scores candidates using job and interview data; plans to integrate with existing systems for workflow automation.

- **Credentials sharing:** Bullhorn API keys and Device API access to be shared; GitHub access granted for development progress.

- **Team alignment:** MVP launch prioritized for job search/contact pages; ongoing feedback and training planned to support rapid deployment.

## Transcript

Tony: Hi, Chris.
Tony: Hey, how's it going?
Tony: Yeah, good, thank you.
Tony: Yourself?
Tony: Pretty good, Pretty well excellent.
Tony: Is it early over there for you or is it.
Tony: Not too bad?
Chris Sterry: It's 7am not too bad.
Tony: Okay.
Tony: First call of the day.
Chris Sterry: No.
Tony: Oh, really?
Chris Sterry: Yeah, I start early, so yeah.
Tony: Oh, fair enough.
Tony: Strong coffee in there, is it?
Chris Sterry: Yeah, the espresso machine downstairs works.
Chris Sterry: I might in the middle of the call, need to go get another.
Chris Sterry: But.
Tony: Yeah, brilliant.
Tony: So you normally start early and finish it slightly early or what's the.
Chris Sterry: I finish when I finish, but yeah, I usually start early and then depending on the day, know it's.
Chris Sterry: It could be long days, it could.
Tony: Be shorter days, but yeah, see how it goes.
Tony: Nice.
Chris Sterry: You?
Leah Smith: Yeah, good, thank you.
Leah Smith: How are you?
Leah Smith: How are you, Chris?
Luke Marsden: Doing well, how about you?
Leah Smith: Yeah, good, thank you.
Leah Smith: Very well.
Chris Sterry: Luke should be.
Chris Sterry: I just got off a call with him.
Chris Sterry: He should be here in just a minute.
Tony: That's right, yeah.
Tony: Nice.
Tony: So.
Tony: So where about.
Luke Marsden: See you, Chris.
Tony: In the US again, I'm sure.
Chris Sterry: Oregon.
Tony: Oregon, yeah.
Tony: Do you have much World Cup.
Tony: World cup fever there at the moment or.
Chris Sterry: Yeah, you know, a little bit.
Chris Sterry: Yeah, I enjoy watching it.
Chris Sterry: I was camping this weekend and got a little bit of service when I was able to watch it on my phone.
Tony: Oh, nice.
Tony: There's no.
Tony: No games in Oregon, is there?
Tony: Is there?
Luke Marsden: No, this meeting is being recorded.
Luke Marsden: Hey, folks.
Luke Marsden: Yeah, good, thanks.
Luke Marsden: How are you?
Tony: Yeah, very good, thank you.
Luke Marsden: Good.
Luke Marsden: How are your weekends?
Tony: Yeah, yeah, good, yeah, it's fine.
Tony: Yeah.
Tony: How about yourself?
Luke Marsden: Yeah, good.
Luke Marsden: Put on a festival with a whole bunch of mates.
Tony: Oh, nice.
Luke Marsden: And yeah, it was great fun.
Tony: So does that entail.
Luke Marsden: Then we rented this like venue that's like a farm that has a massive barn and lots of nice outdoor space and all rocked up and.
Luke Marsden: Yeah, they set up a big system and just had lots of music.
Tony: Are you in a band or something like that?
Tony: Are you or.
Luke Marsden: No, it's.
Luke Marsden: I'm not and it.
Luke Marsden: I didn't dj, but it was like mostly electronic stuff.
Tony: Oh, nice.
Tony: Yeah.
Tony: Invite me to the next one.
Tony: That sounds awesome.
Tony: Well, yeah, very different to that one.
Tony: Was like kids football tournaments and stuff.
Luke Marsden: Like that, but fair enough.
Luke Marsden: Yeah, that's my next weekend.
Tony: Oh, is it?
Luke Marsden: Yeah, yeah, nice.
Luke Marsden: Cool.
Luke Marsden: So I have something to show you.
Luke Marsden: Get some initial feedback.
Luke Marsden: This is just a starting point.
Luke Marsden: Let me just.
Tony: I haven't had a chance to set up the GitHub yet, by the way, so I'll do that.
Luke Marsden: Oh yeah, don't worry.
Tony: When I get a minute.
Luke Marsden: Yeah, that's fine.
Luke Marsden: So this is really just kind of a framework that we can start putting content inside.
Luke Marsden: But yeah there's a navigation here, simple contact and then different pages.
Luke Marsden: So homepage it's got explore roles, hire talent that takes you through to a job search same as this jobs page there work with us.
Luke Marsden: And then these just don't have anything in them yet really.
Luke Marsden: They've just got like a little bit of text about findai.
Luke Marsden: Community, Insights and Contact which we took just from the kind of the basic structure of Linuxrecruit.co.uk.
Luke Marsden: So I think that the.
Luke Marsden: Well a. I'll start by just saying feedback so far please.
Luke Marsden: Anything immediate that you have?
Tony: Yeah, yeah all looks.
Tony: All looks good.
Tony: The.
Tony: The insights bit.
Tony: Would you kind of automate stuff to go in there somehow?
Tony: Is that.
Tony: Is that it's like general AI insights or what.
Tony: What would you.
Luke Marsden: Oh for insights.
Tony: Yeah could you could probably do that.
Tony: Couldn't you have like almost like a live news thing?
Luke Marsden: Yeah yeah.
Tony: Being.
Tony: Being pulled through every.
Tony: Every day or every week or whatever from somewhere at the moment.
Tony: We manually do the kind of content hub kind of thing and that.
Tony: That's.
Tony: That looks like the community for.
Tony: For this new one.
Tony: I'm just wondering if Insights.
Tony: But it's mostly just events isn't it Leah, these days?
Leah Smith: Yeah, yeah.
Tony: Pre and post event stuff.
Tony: Yeah.
Tony: To really chuckle that in community.
Luke Marsden: Okay.
Luke Marsden: Interesting.
Luke Marsden: Yeah.
Luke Marsden: I mean we can do anything you like really.
Luke Marsden: I think before we go off and build like a whole new thing for automatically populating content it's probably worth just focusing on the.
Tony: Yeah, yeah.
Tony: I think I was just thinking out loud really.
Luke Marsden: Yeah, yeah yeah.
Luke Marsden: But it is possible in the future and.
Luke Marsden: But I imagine there's.
Luke Marsden: You're probably running some events that will go on both sites in the future.
Tony: Yeah, I think we.
Tony: We will put them on.
Tony: We'll probably duplicate them.
Tony: Le.
Tony: I would have thought we want to keep the LC brand and keep stuff going on there and people going to that as well as a new one.
Tony: They are.
Tony: Although it's the same.
Tony: We're kind of trying to keep them slightly separate at the same time if that makes sense.
Luke Marsden: Yeah.
Luke Marsden: Yeah.
Chris Sterry: Cool.
Luke Marsden: Okay.
Luke Marsden: Yes.
Luke Marsden: I think.
Luke Marsden: I think top level are there.
Luke Marsden: Thank you.
Luke Marsden: I think top level are there.
Luke Marsden: Are these the right headings?
Luke Marsden: The right main pages?
Luke Marsden: Maybe we just take Insights out for now if it's gonna.
Luke Marsden: Yeah, I'm not sure like I was.
Tony: Actually just thinking I built something really on replit that was basically.
Tony: I was just seeing what it could do really.
Tony: And it was.
Tony: It basically pulls out or I Saw a list of like 50 engineering tech blogs which Google and Amazon, Facebook and you know, all the companies have good engineering tech blogs and it basically pulled in all of the blogs from all those places and updated every time a new one was put on there.
Tony: But also pulled some kind of insights from each of those blogs about recurring themes and top notes, stuff like that.
Tony: It was really, it was quite, it was quite, quite clever.
Tony: But it's just an automatic thing that you know, automatically populated.
Tony: They just having to touch it for something that automatic populating might be good.
Tony: I don't know, it might not.
Luke Marsden: Yeah, yeah, yeah, no that's cool.
Luke Marsden: And I'm thinking we're doing a project with the Linux foundation and they have a lot of the, the same requirements for.
Luke Marsden: I mean they do like an automatic newsletter generation where it's like human curated but the AI goes and finds like a lot of the inputs so it makes it much faster to get the newsletter out every week.
Luke Marsden: That's the same kind of thing that could feed in here.
Luke Marsden: So I'm might be able to reuse because they're doing it with some open source tech.
Luke Marsden: It might be quite good if we could reuse that here.
Luke Marsden: So I'll have a think about that.
Tony: Okay, cool.
Tony: And on the job search function is that obviously talked about the AI side of things with that.
Tony: I guess that's going to be like the phase two where people are able to upload a CV and it automatically stuff and that kind of stuff.
Tony: Yeah, okay, cool.
Luke Marsden: Yeah, yeah, exactly.
Luke Marsden: So I mean the way I've set this up is we've got the whole, I mean this is just the code for it, but we've got the whole infrastructure for a back end in here as well as the front end that's already in place.
Luke Marsden: So we can start filling that in now.
Luke Marsden: So yeah, in terms of like user registration login, like what's the, what's the most important thing to work on next from your perspective?
Tony: Well, I guess now we've got something at least we can start you know, prospecting from Find AI if it makes sense because it's somewhere for them to land.
Tony: I guess we probably need like a temporary solution.
Tony: So when they click on job search now, do you think we should maybe point them to Linux recruit job page or something like that?
Tony: I don't know what the best.
Luke Marsden: Yeah, what goes here?
Luke Marsden: It's like, okay, it's like this.
Luke Marsden: Yeah.
Luke Marsden: And this connects.
Luke Marsden: How is this hosted at the moment?
Tony: So we've, we've got like a basic kind of cms.
Tony: Yeah.
Tony: Bold boulders who built it and they.
Tony: Yeah, they've got like a basic CMS that Leah goes on and can add adverts and stuff manually.
Luke Marsden: Okay.
Luke Marsden: Would you want it to hook up to your.
Luke Marsden: Sorry, go ahead.
Tony: Yeah, in terms of hosting, they host it.
Tony: I'm just wondering if there's a way that we can pull the AI jobs across like temporarily from this page to find AI.
Leah Smith: Yeah.
Leah Smith: Because the AI roles are.
Leah Smith: I don't know if this helps, but they are.
Leah Smith: All of the jobs are tagged on under specialism, so that will make it easier to.
Luke Marsden: Okay, cool.
Tony: Yeah, yeah, yeah, we're just able to pull those across.
Tony: But then I guess if they wanted to apply they'd have to go to Lynch Creed maybe.
Tony: I. I don't know.
Luke Marsden: Yeah, I mean I think doing an integration with your database would probably be a sensible next step because we're going to need that anyway.
Luke Marsden: Yeah, for sure.
Luke Marsden: Like for the agents to be able to.
Luke Marsden: To access it and stuff.
Luke Marsden: So.
Luke Marsden: Um.
Luke Marsden: Yeah.
Luke Marsden: Was there, was there a way for you to get an API key that we could use to log into?
Luke Marsden: Yeah.
Luke Marsden: What was the system called again?
Luke Marsden: Sorry?
Tony: Bullhorn.
Luke Marsden: It's called Bullhorn.
Luke Marsden: Okay, cool.
Tony: Yeah, I think.
Tony: Have we got that already?
Leah Smith: Yeah, I remember seeing it somewhere.
Tony: Yeah.
Tony: Recently as well.
Luke Marsden: Yeah.
Luke Marsden: Cool.
Luke Marsden: If you would just drop that on Slack for, for me please then and we can get.
Luke Marsden: Get this plugged in and.
Luke Marsden: Yeah, I think surfacing.
Luke Marsden: I mean, what's the simplest.
Luke Marsden: Would the simplest thing be to kind of replicate this functionality but just restricted AIML initially or even.
Luke Marsden: Or would even simpler be.
Luke Marsden: I mean, would it.
Luke Marsden: Could we.
Luke Marsden: Could we simplify it further than this?
Luke Marsden: Like maybe you don't do alerts to begin with and things.
Tony: Yeah, yeah, yeah, definitely.
Tony: I don't know how many people have set up alerts actually.
Tony: Can you see that in the back end actually, Leah?
Leah Smith: No, it's not.
Tony: No, just a bit poor really.
Tony: That's one thing that we probably should have visibility on.
Tony: Yeah, I mean just, just putting those jobs across and making it really basic initially.
Tony: But obviously the way is to make it more of like an AI search so people can upload a CV or create like a mini profile that's searchable from.
Tony: By clients.
Luke Marsden: Yeah.
Tony: As well.
Tony: And it pushes jobs to them obviously if they want that they can, they can maybe opt out of that or whatever or they don't have to.
Tony: They can just apply to one.
Tony: One job if they wish.
Luke Marsden: Yeah.
Luke Marsden: And that's the kind of Jack and Jill, the mini Jack and Jill piece, right?
Tony: Yeah, yeah, yeah.
Luke Marsden: Okay, cool.
Tony: In the interim.
Tony: Yeah, if whatever's, you know, simple and easy really, to be honest.
Tony: And even the work with us, we can put some basic copy on there.
Tony: Yeah, but then they can just have a Contact Us page almost rather than, you know, anything elaborate on there.
Tony: We can.
Tony: In the copy that we send you, we can maybe put that, you know, there is an AI search facility being built at the moment for now or maybe be an early, early subscriber or early register your interest early with us or something like that.
Tony: What's the, what's the correct term?
Tony: Be, you know, part of the pilot scheme or whatever.
Tony: Sure.
Tony: Early access.
Tony: Early access, yeah.
Luke Marsden: I like that.
Leah Smith: Yeah.
Luke Marsden: Okay, great.
Luke Marsden: Yeah, I can, I can work on, on building these out.
Luke Marsden: And, and in terms of the copy, do you want me to start by kind of for the other pages, do you want me to start by kind of cloning this or do you want to send me.
Tony: Yeah, yeah, I feel like we might start from, from scratch, personally.
Luke Marsden: Okay, sure.
Tony: Again, we might simplify and you know, keep the copy quite short and concise.
Tony: I don't know if you'd agree with that.
Leah Smith: I think there's a few.
Leah Smith: Yeah, we want to simplify it.
Leah Smith: So we do want to keep, keep.
Leah Smith: I think, Tony, the, the logos and like clients that we work with and.
Leah Smith: But I think on the Linux Recruit website about kind of embedded recruitment.
Leah Smith: I'm not sure, Tony, I don't know if you think.
Leah Smith: Would we go ahead with mentioning all that kind of stuff or just keep it really simple.
Tony: I just keep it super simple, basic for now.
Tony: Obviously there's going to be bit of work about the Mini Jack and Jill stuff once we, once we get.
Tony: Get going.
Tony: But yeah, definitely for now, almost like a.
Tony: It's obvious what.
Tony: We'll make it clear what we do in.
Leah Smith: I think as long as there's clear call to actions as well.
Leah Smith: So at the bottom of the pages like we have on Linux Recruit, if you're looking to hire, if you want to search for your next role, get in touch.
Leah Smith: Just so that there's like a touch point at the end of every page.
Luke Marsden: Yeah, yeah, that sounds good.
Luke Marsden: Cool.
Luke Marsden: And the other thing I'll try and get done in the next few days is making it so that you can have access to an AI that you can use to modify the website and update it yourself.
Luke Marsden: And then things like that new copy that you want to add and so on you'll be able to do through that system.
Luke Marsden: And we can probably set up.
Luke Marsden: I mean, maybe we can do it next week actually in our Usual call, kind of just do, do it, do some training for you on using that system and, and try pushing a few updates through.
Leah Smith: I'm actually, I'm off next Monday.
Leah Smith: I just wondered for, just for next Monday.
Leah Smith: I think I might be off the following Monday, but if there's.
Leah Smith: For the next two.
Leah Smith: If I look at my company maybe for next week, if we could.
Leah Smith: If you're free on the Tuesday or something, I'd like part of.
Leah Smith: Yeah, I think that'd be beneficial.
Luke Marsden: No, no, definitely should keep you in the loop earlier.
Luke Marsden: That sounds good.
Luke Marsden: Of course.
Luke Marsden: Okay, great.
Luke Marsden: Okay, cool.
Luke Marsden: I mean, I think I need.
Luke Marsden: I know what I need to do next.
Luke Marsden: So was there anything else, any other topics?
Tony: Just I just have one.
Tony: One more question on the agents, actually.
Tony: Yeah, this is a couple of.
Tony: Couple of things that we've been.
Tony: Been doing internally in the last week or so and that.
Tony: That's us kind of clarifying candidate suitability to a role.
Tony: Yeah, we've been doing it on, on chat GPT.
Tony: All I've done is pulled in job descriptions, interview feedback, notes, details from people who work there, transcript of briefings, all that kind of stuff.
Tony: Congratulation.
Tony: All of the info on a, on a roll.
Tony: And then we've been dragging CVS into that along with transcripts and calls saying, what do you think?
Tony: I mean, we should know suitability.
Tony: But for some of these really, really niche roles, suitability giving us like a bit of a score and what we need to clarify more on.
Tony: So it was helping the guys.
Tony: So I don't know, I feel like that would be super easy for, you know, if I could give the guys a tool that is basically they can drag and drop like recordings and notes and all this kind of stuff and then put TV there, then notes or a transcript from their call with that person and then it pushes out maybe a score or maybe what they're lacking or whatever.
Luke Marsden: Yeah, yeah, yeah, yeah.
Tony: That would be really useful for us if that's possible.
Luke Marsden: I really like that.
Luke Marsden: And that kind of touches on the internal efficiency gains piece and it sounds like you're basically prototyping systems that you'd want to build into like Find OS or whatever.
Luke Marsden: We call it like the internal tool.
Luke Marsden: So.
Luke Marsden: Yeah, I really like that one.
Luke Marsden: As a starting point, what system do you use for call recordings?
Luke Marsden: Is that always the same or.
Chris Sterry: Yeah, we use.
Tony: We've got a phone like a VoIP system called device.
Luke Marsden: Okay.
Tony: So you could probably plug into that somehow as well, I imagine.
Tony: I mean, just get on from, from what I just said, actually it could.
Tony: Could get to a point where if we get a new role, we put an advert up and in the back end as long along with that advert, we put all the info about that role.
Tony: So when someone applies, we get an application with a suitability score based on.
Tony: So we've got for that candidate.
Tony: That makes sense that I would be awesome as well.
Tony: Forward.
Tony: I can send you the chat GPT.
Tony: You can kind of share the chat thing, can't you?
Luke Marsden: Sorry.
Luke Marsden: Yeah, yeah, that'd be really helpful.
Luke Marsden: Yeah.
Luke Marsden: Just show me how you're using it now.
Luke Marsden: Yeah, yeah, yeah, yeah.
Luke Marsden: No, sounds great.
Luke Marsden: Cool.
Luke Marsden: Okay.
Luke Marsden: Yeah, awesome.
Luke Marsden: All right.
Leah Smith: It looks really good though.
Leah Smith: Thank you.
Luke Marsden: Yeah, of course.
Luke Marsden: I mean, it's just a starting point, but I'm glad that we got the website in dev.
Luke Marsden: I'll get it online as well, actually, on the domain itself with your 123 login and we can chat on Slack just to make sure that you're happy with it before it goes live.
Tony: Sounds good.
Tony: We'll get the GitHub account this afternoon.
Tony: Yep, yep.
Luke Marsden: Yeah, sorry.
Leah Smith: And did you.
Leah Smith: Shall I investigate if we can get the device API logins as well?
Leah Smith: If that's the thing they do.
Luke Marsden: Yeah, yeah, yeah, that would be good.
Luke Marsden: And I. I Googled for device VoIP system and just didn't get any answers, so.
Tony: U E V Y C E. Ah, that's why.
Chris Sterry: Yeah.
Luke Marsden: Okay, cool.
Luke Marsden: Got it.
Tony: It pushes out like that also links to bullhorn funnily enough, so it automatically populates bullhorn with notes.
Luke Marsden: Oh, it does already.
Luke Marsden: Okay.
Luke Marsden: Oh, we probably don't need to integrate directly with device then.
Luke Marsden: Yeah,.
Tony: Populate.
Tony: Say like an AI.
Tony: It kind of does it like an AI note taker thing.
Tony: So the call will be put into three or four lines, bullet points and stuff.
Tony: So you won't get the full transcript from that, just the AI notes, but okay.
Tony: Yeah, yeah.
Luke Marsden: Cool.
Luke Marsden: All right, well, we can plug into whatever we need to, so.
Luke Marsden: Sounds good.
Tony: Amazing.
Luke Marsden: Wicked.
Luke Marsden: All right, have a good week then.
Chris Sterry: Cheers.
Luke Marsden: Thanks so much.
Chris Sterry: Bye.

