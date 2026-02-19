# PROJECT SPEC: SPACETRADERS LEADERBOARD MIGRATION

## 1. OBJECTIVE
Migrate the existing file-based leaderboard system to a persistent Turso (libSQL) database. This will enable real-time updates and better data structure for the SpaceTraders.io client.
The current implementation is combination of json and csv writers. It is not in a working state.
I would like to make a replacement for the internal/agentcollector. Instead, it should be called just 'collector'
Logic can be copied from agentcollector and used as required.

---

## 2. TECH STACK
* **Database:** Turso (libSQL)
* **Client SDK:** @libsql/client
* **Backend:** entire project is in Go. The frontend is an htmx system that leans on apache e-charts
* **Data Source:** SpaceTraders.io API

---

## 3. DATABASE SCHEMA (PROPOSED)

### Table: agents
This holds the main data we use
| Column | Type | Description |
| :--- | :--- | :--- |
| timestamp | Integer | timestamp in unix epoche time |
| reset | text | Identifier of which session we are in - format is "2006-01-02"|
| symbol | TEXT (PK) | Unique agent identifier |
| ships | integer | count of ships agent has |
| faction | TEXT | Starting faction |
| credits | INTEGER | Current credit balance |
| headquarters | TEXT | symbol of home world |

### Table: leaderboard
This table will hold records that are tied together by the timestamp and reset
We will get up to 10 records per timestamp for each of credits or charts
| Column | Type | Description |
| :--- | :--- | :--- |
| timestamp | Integer | timestamp in unix epoche time |
| reset | text | Identifier of which session we are in - format is "2006-01-02"|
| count | INTEGER | number of credits or charts at this time |
| symbol | TEXT (PK) | Unique agent identifier |
| type | text | will be one of charts or credits for the two leaderboards

### Table: Jumpgates
This is the jumpgate status table. Each starter system has a jumpgate that needs
to be built. This table relates the system, headquarters, jumpgate, and its construction stats 
together.

| Column | Type | Description |
| :--- | :--- | :--- |
| reset | text | Identifier of which session we are in - format is "2006-01-02"|
| system | text | name of the systm |
| headquarters | text | headquarters in that system |
| jumpgate | text | symbol of the jumpgate |
| complete | integer | timestamp of when construction complete, in UTC unix epoch, zero if not done |
| activeagent | bool | is there an active agent in this system |

### Table: Construction

| Column | Type | Description |
| :--- | :--- | :--- |
| reset | text | Identifier of which session we are in - format is "2006-01-02"|
| timestamp | Integer | timestamp in unix epoche time |
| jumpgate | text | symbol of the jumpgate links to jumpgates table |
| fabmat | integer | number of fabmats delivered to the jumpgate |
| advcct | integer | number of advanced circuitry units delivered |

### Table stats
This is a remainder of fields that can be stored, will be updated without timestamps
It is fed from the status endpoint at '/'

| Column | Type | Description |
| :--- | :--- | :--- |
| reset | text | Record of the latest reset - format is "2006-01-02"|
| marketUpdate | datetime | timestamp of the latest market update |
| agents | integer | number of registered agents |
| accounts | integer | number of accounts registered |
| ships | integer | number of ships |
| systems | integer | number of systems |
| waypoints | integer | number of waypoints |
| status | text | status string |
| version | text | version string |
| nextReset | datetime | time when system will reset next |



## 4. FUNCTIONAL REQUIREMENTS

### Current layout
* This application works in a container on a cloud hosting platform (railway)
* main.go starts the App
* the App starts the collector that runs in a go func
* the App continues to run to serve pages for the htmx frontend
* the collector does an ingestion every 5 minutes.
    * this includes the leaderboard call and agents
    * there was some work to only run jumpgate ingestion on 15 or hour long intervals
      but that should be ignored and all ingestion runs every 5 minutes
* in order to reduce cost, cpu and memory should be minimized. The frontend should zero
  out its data structures so memory can be reclaimed by the built in GC
  prefer to have more complex queries instead of manipulating data in code.
* along same lines try to batch a number of database updates
* system can be found from headquarters by removing the last three characters (usually '-A1')

### Data Ingestion (The "Bridge")
* Create a service that reads current SpaceTraders API responses.
* Instead of writing to `.json` files, perform an **UPSERT** to the Turso database.
* **Reference Logic:** is stored in the current agentcollector package
* api spec is here: https://github.com/SpaceTradersAPI/api-docs 
    * reference directory has most of it with models in the models directory

### Calls used
All of these calls are relative to the base url of 'https://api.spacetraders.io/v2'

#### / 
* the status/health call
* this will be done first
* it can set the data in the `stats` table
* can keep reset date in a variable as it will be used later on
* leaderboard will be updated, 
  * for each of the objects in mostCredits, we add a row with the type of 'credit'
  * for each of the objects in mostSubmittedCharts, we add a row of type 'charts'

#### /agents
* loop through all the agents as many pages as needed
* max of 20 per call
* data is written to the agents table

#### /systems/<system>
* will give all the waypoints in the system with abbreviated details
* will use this to find the symbol of the jumpgate

#### /systems/<system>/waypoints/<jumpgatesymbol>/construction
* can tell us if construction is complete
* also shows how many materials have been delivered for construction

### Query Logic
* save timestamp so all records are the same (int64 UTC epoch) 
* call status/health endpoint
    * update all fields we can in stats and leaderboard tables
    * store reset date as we will use that a lot as well
* reset date will be used for all db calls that have that in the table
* call agents endpoint
    * for each agent:
        * add to agents table
        * is headquarters in jumpgate table?
            * no -> add it with system 
                * if user credits is not 175000, set activeagent to true
            * yes -> if there is a complete time, done,move to next agent
                * if not complete and agent is not 175k, set activeagent to true
* bring in all jumpgates that are not complete
    * for each one, call the construction endpoint for it
    * add rows for fabmats and advcct fields for each jumpgate inthe construction table
    * if complete was zero and the construction endpoint says isComplete - set timestamp in complete for that jumpgate


---

## 5. REFACTORING STRATEGY
* **Phase 1:** Setup Turso connection and initialize Schema.
* **Phase 2:** Replace the "Save to Disk" functions with a "Save to DB" functions.
* **Phase 3:** Update Frontend hooks to fetch from DB instead of reading local files.
