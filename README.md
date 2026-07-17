# Project Hawaii

An program that takes replays in and uses AI to analyze the replay, and build a database about playstyles of teams/clans. Works best with clan battles where you can associate clans between many battles over time.

## Project Structure

This project is built with a server and a client. The server processes replays, stores data, and also secures it with a local accounts database. The client is a web application that allows users to view the data and analyze it.

**Accounts**: The server has a local accounts database that allows users to create accounts and log in. This is used to secure the data and ensure that only authorized users can access it. Accounts are basic username/password accounts. The first account created is an administrator account. Administrative accounts can determine sign-up policy (open, closed, invite-link-only).

**Replay Processing**: The server processes replays and builds a time-series of data about the battle. This data can be converted to a video on the client, but is not stored as a video on the server.

**AI Analysis**: The server uses AI to analyze the replay data and infer playstyles, calls, value, and other information about the teams/clans. This data is stored in a database and can be queried by clients. This is configured with an OpenAPI, so self-hosted LM Studio or other LLMs can be used instead of OpenAI.

---

**User Stories**:

- As a casual user, I want to be able to upload a replay of a battle I had, and get an analysis of the playstyles of myself and the teams involved, so that I can improve my own gameplay.
- As a clan caller, I want to be able to upload replays of my clan battles, and get an analysis of the playstyles of my team and the opposing team, so that I can improve our strategies and calls.
- As any type of user, I should be able to browse the database of clans and their playstyles, so I can learn from other teams and improve my own gameplay, and prepare to fight against them in future battles.
- As a clan battle caller, I want to be able be able to get a recommandation of what an enemy team may do based on the current map, their clan, and their composition.
- As an administrator, I want to be able to manage user accounts, uploaded replays, and manage the database to ensure the system is running smoothly.

## Tech Stack

- **Backend**: Go, SQLite, Go Fiber, OpenAPI (LM Studio)
- 