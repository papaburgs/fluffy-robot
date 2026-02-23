On collections - we can do this every 5 mins

### status https://api.spacetraders.io/v2/
  * returns:
    * reset date
    * leaderboard
    * status (which doesn't have much use)
    * health: { lastMarketUpdate: "2026-02-18T03:02:16.222Z" }
      * should be less than 2 mins from now

### public agents https://api.spacetraders.io/v2/agents
   {
      "symbol": "string",
      "headquarters": "string",
      "credits": 1,
      "startingFaction": "string",
      "shipCount": 1
    }

For each agent where credits != 175000:

do a call for home system:
### system: https://api.spacetraders.io/v2/systems/X1-XXXX
    * this will give all waypoints in the system
    * look for jumpgate:
        loop over waypoints struct for:
           {
        orbitals: []
        symbol: "X1-BR56-I57"
        type: "JUMP_GATE"
        x: -389
        y: -221
      }
    * we save the jump gate symbol so we don't have to look up again
    * note we have a lookup of system, homeworld and jumpgate

### waypoints https://api.spacetraders.io/v2/systems/X1-XXXX/waypoints/X1-XXXX-I57/construction
can use this instead of one above if we don't know the waypoint symbol of the jumpgate, but we only get 10 at a time, so better to do it once to find it

but can add the jumpgate symbol to the end to get if it still under construction

### contruction https://api.spacetraders.io/v2/systems/X1-XXXX/waypoints/X1-XXXX-I57/construction
This will give totals of what's left
{
  data: {
    isComplete: false
    materials: [
      {
        fulfilled: 1000
        required: 1600
        tradeSymbol: "FAB_MATS"
      }
      {
        fulfilled: 380
        required: 400
        tradeSymbol: "ADVANCED_CIRCUITRY"
      }
      {
        fulfilled: 1
        required: 1
        tradeSymbol: "QUANTUM_STABILIZERS"
      }
    ]
    symbol: "X1-BR56-I57"
  }
}

so will take a few calls, but can make one row per system,
homeworld
system
isActive (someone has something other than 175k
jumpgate
isComplete
Fabmats fulfilled
Advanced Circtu fulfilled

then we can join each player to their system through the tables.



